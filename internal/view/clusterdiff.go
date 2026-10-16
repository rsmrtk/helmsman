// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"context"
	"fmt"
	"sort"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// workload is what :diff compares: a controller, its images and replica counts.
type workload struct {
	Key      string // kind/namespace/name
	Images   []string
	Replicas string // ready/desired
}

func images(spec v1.PodSpec) []string {
	var ii []string
	for _, c := range spec.Containers {
		ii = append(ii, c.Image)
	}
	sort.Strings(ii)

	return ii
}

func fetchWorkloads(ctx context.Context, name, ns string) (map[string]workload, error) {
	cs, err := clientFor(name)
	if err != nil {
		return nil, err
	}
	out := map[string]workload{}
	add := func(kind, namespace, n string, spec v1.PodSpec, ready, want int32) {
		k := fmt.Sprintf("%s/%s/%s", kind, namespace, n)
		out[k] = workload{Key: k, Images: images(spec), Replicas: fmt.Sprintf("%d/%d", ready, want)}
	}
	opts := metav1.ListOptions{}
	dp, err := cs.AppsV1().Deployments(ns).List(ctx, opts)
	if err != nil {
		return nil, err
	}
	for _, d := range dp.Items {
		want := int32(1)
		if d.Spec.Replicas != nil {
			want = *d.Spec.Replicas
		}
		add("deploy", d.Namespace, d.Name, d.Spec.Template.Spec, d.Status.ReadyReplicas, want)
	}
	if sts, err := cs.AppsV1().StatefulSets(ns).List(ctx, opts); err == nil {
		for _, s := range sts.Items {
			want := int32(1)
			if s.Spec.Replicas != nil {
				want = *s.Spec.Replicas
			}
			add("sts", s.Namespace, s.Name, s.Spec.Template.Spec, s.Status.ReadyReplicas, want)
		}
	}
	if ds, err := cs.AppsV1().DaemonSets(ns).List(ctx, opts); err == nil {
		for _, d := range ds.Items {
			add("ds", d.Namespace, d.Name, d.Spec.Template.Spec, d.Status.NumberReady, d.Status.DesiredNumberScheduled)
		}
	}

	return out, nil
}

type diffRow struct {
	Key, A, B, Kind string // Kind: only-a, only-b, differs, same
}

func imageList(w workload) string {
	return strings.Join(w.Images, ",") + " (" + w.Replicas + ")"
}

// diffWorkloads pairs the two sides by key and classifies each.
func diffWorkloads(a, b map[string]workload) []diffRow {
	keys := map[string]struct{}{}
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	var rows []diffRow
	for k := range keys {
		wa, oka := a[k]
		wb, okb := b[k]
		r := diffRow{Key: k}
		switch {
		case oka && !okb:
			r.Kind, r.A = "only-a", imageList(wa)
		case !oka && okb:
			r.Kind, r.B = "only-b", imageList(wb)
		case strings.Join(wa.Images, ",") != strings.Join(wb.Images, ",") || wa.Replicas != wb.Replicas:
			r.Kind, r.A, r.B = "differs", imageList(wa), imageList(wb)
		default:
			r.Kind, r.A, r.B = "same", imageList(wa), imageList(wb)
		}
		rows = append(rows, r)
	}
	order := map[string]int{"differs": 0, "only-a": 1, "only-b": 2, "same": 3}
	sort.Slice(rows, func(i, j int) bool {
		if order[rows[i].Kind] != order[rows[j].Kind] {
			return order[rows[i].Kind] < order[rows[j].Kind]
		}
		return rows[i].Key < rows[j].Key
	})

	return rows
}

func diffText(ca, cb, ns string, rows []diffRow) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s%s%s  vs  %s%s%s   %snamespace %s%s\n\n", statusTag("Running"), ca, tagReset, statusTag("Completed"), cb, tagReset, tagDim, ns, tagReset)
	same := 0
	for _, r := range rows {
		switch r.Kind {
		case "same":
			same++
		case "differs":
			fmt.Fprintf(&sb, "%s≠ %s%s\n    %s%s: %s%s\n    %s%s: %s%s\n", statusTag("NoReady"), r.Key, tagReset, tagDim, ca, tagReset, r.A, tagDim, cb, tagReset, r.B)
		case "only-a":
			fmt.Fprintf(&sb, "%s◀ %s%s  only in %s  %s\n", statusTag("Running"), r.Key, tagReset, ca, r.A)
		case "only-b":
			fmt.Fprintf(&sb, "%s▶ %s%s  only in %s  %s\n", statusTag("Completed"), r.Key, tagReset, cb, r.B)
		}
	}
	if len(rows) == same {
		fmt.Fprintf(&sb, "%sno differences%s\n", statusTag("Running"), tagReset)
	}
	fmt.Fprintf(&sb, "\n%s%d identical, %d to look at%s\n", tagDim, same, len(rows)-same, tagReset)

	return sb.String()
}

// diffCmd compares the workloads of a namespace across two clusters:
// ":diff ctxA ctxB [namespace]". The namespace defaults to all.
func (a *App) diffCmd(args string) {
	f := strings.Fields(args)
	if len(f) < 2 {
		a.Flash().Err(fmt.Errorf("use `diff <context-a> <context-b> [namespace]`"))
		return
	}
	ca, cb, ns := f[0], f[1], ""
	if len(f) > 2 && f[2] != "all" {
		ns = f[2]
	}
	label := ns
	if label == "" {
		label = "all"
	}
	a.textScreen("Diff", ca+" vs "+cb, func() string {
		ctx, cancel := context.WithTimeout(context.Background(), 2*multiTimeout)
		defer cancel()
		wa, err := fetchWorkloads(ctx, ca, ns)
		if err != nil {
			return fmt.Sprintf("%s: %v", ca, err)
		}
		wb, err := fetchWorkloads(ctx, cb, ns)
		if err != nil {
			return fmt.Sprintf("%s: %v", cb, err)
		}

		return diffText(ca, cb, label, diffWorkloads(wa, wb))
	})
}
