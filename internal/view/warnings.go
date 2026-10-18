// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// warning is one problem the cluster keeps reporting, merged over repeats.
type warning struct {
	Object, Reason, Message string
	Count                   int32
	Last                    time.Time
}

// reasonStatus maps an event reason onto the palette entry that looks like it.
var reasonStatus = map[string]string{
	"BackOff": "CrashLoopBackOff", "Failed": "Error", "FailedScheduling": "Pending", "FailedMount": "CreateContainerConfigError",
	"FailedCreate": "Error", "Unhealthy": "NoReady", "OOMKilling": "OOMKilled", "Evicted": "Evicted",
	"ErrImagePull": "ImagePullBackOff", "ImagePullBackOff": "ImagePullBackOff", "InvalidImageName": "InvalidImageName",
	"FailedAttachVolume": "CreateContainerConfigError", "NodeNotReady": "CrashLoopBackOff", "Preempted": "Pending",
}

func eventTime(e v1.Event) time.Time {
	switch {
	case !e.LastTimestamp.IsZero():
		return e.LastTimestamp.Time
	case !e.EventTime.IsZero():
		return e.EventTime.Time
	default:
		return e.CreationTimestamp.Time
	}
}

// collectWarnings keeps Warning events, merges repeats of the same object and
// reason, and orders them: most repeated first, then most recent.
func collectWarnings(events []v1.Event) []warning {
	byKey := map[string]*warning{}
	for _, e := range events {
		if e.Type != v1.EventTypeWarning {
			continue
		}
		obj := strings.ToLower(e.InvolvedObject.Kind) + "/" + e.InvolvedObject.Name
		if e.InvolvedObject.Namespace != "" {
			obj = e.InvolvedObject.Namespace + "/" + obj
		}
		key := obj + "|" + e.Reason
		w, ok := byKey[key]
		if !ok {
			w = &warning{Object: obj, Reason: e.Reason}
			byKey[key] = w
		}
		w.Count += max(e.Count, 1)
		if t := eventTime(e); t.After(w.Last) {
			w.Last, w.Message = t, strings.TrimSpace(e.Message)
		}
	}
	out := make([]warning, 0, len(byKey))
	for _, w := range byKey {
		out = append(out, *w)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Last.After(out[j].Last)
	})

	return out
}

func since(t, now time.Time) string {
	d := now.Sub(t).Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}

	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func warningsText(ww []warning, now time.Time) string {
	if len(ww) == 0 {
		return fmt.Sprintf("%s●%s no warnings: nothing is complaining\n", statusTag("Running"), tagReset)
	}
	var sb strings.Builder
	sb.WriteString(heading(pad("COUNT", 6), pad("LAST", 6), pad("REASON", 22), "OBJECT"))
	for _, w := range ww {
		key := reasonStatus[w.Reason]
		tag := statusTag("NoReady")
		if key != "" {
			tag = statusTag(key)
		}
		fmt.Fprintf(&sb, "%s%s %s %s%s %s\n", tag, pad(fmt.Sprint(w.Count), 6), pad(since(w.Last, now), 6), pad(w.Reason, 22), tagReset, w.Object)
		fmt.Fprintf(&sb, "%s%s%s%s\n", strings.Repeat(" ", 36), tagDim, escapeTags(oneLine(w.Message)), tagReset)
	}
	fmt.Fprintf(&sb, "\n%s%d distinct warnings%s\n", tagDim, len(ww), tagReset)

	return sb.String()
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func escapeTags(s string) string {
	return strings.NewReplacer("[", "[[").Replace(s)
}

// warningsCmd lists the Warning events of the active namespace (or all).
func (a *App) warningsCmd() {
	ctxName := a.Config.ActiveContextName()
	ns := a.Config.ActiveNamespace()
	if ns == "all" || ns == "-A" {
		ns = ""
	}
	label := ns
	if label == "" {
		label = "all namespaces"
	}
	a.textScreen("Warnings", ctxName+" · "+label, func() string {
		cs, err := clientFor(ctxName)
		if err != nil {
			return err.Error()
		}
		ctx, cancel := context.WithTimeout(context.Background(), multiTimeout)
		defer cancel()
		list, err := cs.CoreV1().Events(ns).List(ctx, metav1.ListOptions{FieldSelector: "type=Warning"})
		if err != nil {
			return err.Error()
		}

		return warningsText(collectWarnings(list.Items), time.Now())
	})
}
