// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

var argoApps = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}

// driftItem is one resource an Argo CD application manages.
type driftItem struct {
	Ref, Sync, Health, Message string
}

type driftReport struct {
	Name, Sync, Health, Revision, Phase string
	Items                               []driftItem
	Conditions                          []string
}

// parseDrift reads an Argo CD Application object into a report.
func parseDrift(app map[string]any) driftReport {
	str := func(path ...string) string {
		v, _, _ := unstructured.NestedString(app, path...)
		return v
	}
	r := driftReport{
		Name:     str("metadata", "name"),
		Sync:     str("status", "sync", "status"),
		Health:   str("status", "health", "status"),
		Revision: str("status", "sync", "revision"),
		Phase:    str("status", "operationState", "phase"),
	}
	res, _, _ := unstructured.NestedSlice(app, "status", "resources")
	for _, e := range res {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		get := func(k string) string { s, _ := m[k].(string); return s }
		ref := get("kind") + "/" + get("name")
		if ns := get("namespace"); ns != "" {
			ref = ns + "/" + ref
		}
		health, msg := "", ""
		if h, ok := m["health"].(map[string]any); ok {
			health, _ = h["status"].(string)
			msg, _ = h["message"].(string)
		}
		r.Items = append(r.Items, driftItem{Ref: ref, Sync: get("status"), Health: health, Message: msg})
	}
	conds, _, _ := unstructured.NestedSlice(app, "status", "conditions")
	for _, c := range conds {
		if m, ok := c.(map[string]any); ok {
			t, _ := m["type"].(string)
			msg, _ := m["message"].(string)
			r.Conditions = append(r.Conditions, t+": "+strings.TrimSpace(msg))
		}
	}
	// the resources that need attention first
	sort.SliceStable(r.Items, func(i, j int) bool { return driftRank(r.Items[i]) > driftRank(r.Items[j]) })

	return r
}

func driftRank(i driftItem) int {
	switch {
	case i.Health == "Degraded" || i.Health == "Missing":
		return 3
	case i.Sync == "OutOfSync":
		return 2
	case i.Health == "Progressing":
		return 1
	}

	return 0
}

// argoTag colours an Argo CD sync or health word through the pod palette.
func argoTag(word string) string {
	switch word {
	case "Synced", "Healthy":
		return statusTag("Running")
	case "OutOfSync":
		return statusTag("NoReady")
	case "Progressing", "Suspended":
		return statusTag("Pending")
	case "Degraded", "Missing":
		return statusTag("CrashLoopBackOff")
	}

	return statusTag("Unknown")
}

func shortRev(s string) string {
	if len(s) > 8 {
		return s[:8]
	}

	return s
}

func driftText(r driftReport) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s%s%s  sync %s%s%s  health %s%s%s  revision %s  last operation %s\n\n",
		tagHeading, r.Name, tagReset, argoTag(r.Sync), r.Sync, tagReset, argoTag(r.Health), r.Health, tagReset, shortRev(r.Revision), firstNonEmpty(r.Phase))
	bad := 0
	for _, i := range r.Items {
		if driftRank(i) > 1 {
			bad++
		}
	}
	if bad == 0 {
		fmt.Fprintf(&sb, "%s●%s no drift: every managed resource matches git and is healthy\n\n", statusTag("Running"), tagReset)
	} else {
		fmt.Fprintf(&sb, "%s%d of %d resources need attention%s\n\n", tagHeading, bad, len(r.Items), tagReset)
	}
	sb.WriteString(heading(pad("SYNC", 11), pad("HEALTH", 12), "RESOURCE"))
	for _, i := range r.Items {
		tag := statusTag("Running")
		switch driftRank(i) {
		case 3:
			tag = statusTag("CrashLoopBackOff")
		case 2:
			tag = statusTag("NoReady")
		case 1:
			tag = statusTag("Pending")
		}
		fmt.Fprintf(&sb, "%s%s %s %s%s\n", tag, pad(firstNonEmpty(i.Sync), 11), pad(firstNonEmpty(i.Health), 12), i.Ref, tagReset)
		if i.Message != "" && driftRank(i) > 0 {
			fmt.Fprintf(&sb, "%s%s%s%s\n", strings.Repeat(" ", 25), tagDim, escapeTags(oneLine(i.Message)), tagReset)
		}
	}
	for _, c := range r.Conditions {
		fmt.Fprintf(&sb, "\n%s%s%s", statusTag("NoReady"), escapeTags(oneLine(c)), tagReset)
	}

	return sb.String() + "\n"
}

// driftCmd shows what an Argo CD application manages and which of it differs
// from git: ":drift <app> [namespace]" (namespace defaults to argocd).
func (a *App) driftCmd(args string) {
	f := strings.Fields(args)
	if len(f) == 0 {
		a.Flash().Errf("use `drift <application> [namespace]`")
		return
	}
	name, ns := f[0], "argocd"
	if len(f) > 1 {
		ns = f[1]
	}
	ctxName := a.Config.ActiveContextName()
	a.textScreen("Drift", ns+"/"+name, func() string {
		cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: ctxName}).ClientConfig()
		if err != nil {
			return err.Error()
		}
		dc, err := dynamic.NewForConfig(cfg)
		if err != nil {
			return err.Error()
		}
		ctx, cancel := context.WithTimeout(context.Background(), multiTimeout*time.Duration(1))
		defer cancel()
		obj, err := dc.Resource(argoApps).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Sprintf("%s✖ %s%s\n", statusTag("CrashLoopBackOff"), err, tagReset)
		}

		return driftText(parseDrift(obj.Object))
	})
}
