// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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

	for _, add := range []func(context.Context, kubernetes.Interface, string, map[string]workload){addConfigs, addServices, addIngresses} {
		add(ctx, cs, ns, out)
	}

	return out, nil
}

// fingerprint hashes values so Secrets and ConfigMaps can be compared without
// ever putting a value on screen.
func fingerprint(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil))[:8]
}

func addConfigs(ctx context.Context, cs kubernetes.Interface, ns string, out map[string]workload) {
	if cms, err := cs.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{}); err == nil {
		for _, c := range cms.Items {
			if c.Name == "kube-root-ca.crt" {
				continue
			}
			keys, vals := sortedKeys(c.Data), []string{}
			for _, k := range keys {
				vals = append(vals, k, c.Data[k])
			}
			k := fmt.Sprintf("cm/%s/%s", c.Namespace, c.Name)
			out[k] = workload{Key: k, Images: []string{"keys=" + strings.Join(keys, ","), "content=" + fingerprint(vals...)}, Replicas: "-"}
		}
	}
	if secs, err := cs.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{}); err == nil {
		for _, s := range secs.Items {
			if s.Type == v1.SecretTypeServiceAccountToken {
				continue
			}
			keys, vals := []string{}, []string{}
			for k := range s.Data {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				vals = append(vals, k, string(s.Data[k]))
			}
			key := fmt.Sprintf("secret/%s/%s", s.Namespace, s.Name)
			out[key] = workload{Key: key, Images: []string{"keys=" + strings.Join(keys, ","), "content=" + fingerprint(vals...)}, Replicas: "-"}
		}
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return keys
}

func addServices(ctx context.Context, cs kubernetes.Interface, ns string, out map[string]workload) {
	svcs, err := cs.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	for _, s := range svcs.Items {
		var ports []string
		for _, p := range s.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
		}
		sort.Strings(ports)
		k := fmt.Sprintf("svc/%s/%s", s.Namespace, s.Name)
		out[k] = workload{Key: k, Images: []string{"type=" + string(s.Spec.Type), "ports=" + strings.Join(ports, ",")}, Replicas: "-"}
	}
}

func addIngresses(ctx context.Context, cs kubernetes.Interface, ns string, out map[string]workload) {
	ings, err := cs.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	for _, i := range ings.Items {
		var hosts []string
		for _, r := range i.Spec.Rules {
			hosts = append(hosts, r.Host)
		}
		sort.Strings(hosts)
		k := fmt.Sprintf("ing/%s/%s", i.Namespace, i.Name)
		out[k] = workload{Key: k, Images: []string{"hosts=" + strings.Join(hosts, ",")}, Replicas: "-"}
	}
}

type diffRow struct {
	Key, A, B, Kind string // Kind: only-a, only-b, differs, same
}

func imageList(w workload) string {
	if w.Replicas == "-" {
		return strings.Join(w.Images, "  ")
	}

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
