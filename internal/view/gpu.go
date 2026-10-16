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

// gpuResources are the extended resources the screen counts as GPUs.
var gpuResources = []v1.ResourceName{"nvidia.com/gpu", "amd.com/gpu", "intel.com/gpu"}

type gpuNode struct {
	Name        string
	Kind        string
	Allocatable int64
	Requested   int64
}

type gpuPod struct {
	Path, Node, Status string
	Count              int64
}

func gpuCount(rl v1.ResourceList) (string, int64) {
	for _, r := range gpuResources {
		if q, ok := rl[r]; ok && !q.IsZero() {
			return string(r), q.Value()
		}
	}

	return "", 0
}

// gpuUsage walks the nodes and pods and returns the GPU nodes and the pods
// that ask for GPUs. Finished pods hold no GPU and are skipped.
func gpuUsage(nodes []v1.Node, pods []v1.Pod) ([]gpuNode, []gpuPod) {
	byName := map[string]*gpuNode{}
	var gn []*gpuNode
	for i := range nodes {
		if kind, n := gpuCount(nodes[i].Status.Allocatable); n > 0 {
			g := &gpuNode{Name: nodes[i].Name, Kind: kind, Allocatable: n}
			byName[g.Name] = g
			gn = append(gn, g)
		}
	}
	var gp []gpuPod
	for i := range pods {
		p := &pods[i]
		if p.Status.Phase == v1.PodSucceeded || p.Status.Phase == v1.PodFailed {
			continue
		}
		var total int64
		for _, c := range p.Spec.Containers {
			_, n := gpuCount(c.Resources.Requests)
			if n == 0 {
				_, n = gpuCount(c.Resources.Limits)
			}
			total += n
		}
		if total == 0 {
			continue
		}
		if g, ok := byName[p.Spec.NodeName]; ok {
			g.Requested += total
		}
		gp = append(gp, gpuPod{Path: p.Namespace + "/" + p.Name, Node: p.Spec.NodeName, Status: podPhase(p), Count: total})
	}
	sort.Slice(gp, func(i, j int) bool { return gp[i].Path < gp[j].Path })
	out := make([]gpuNode, 0, len(gn))
	for _, g := range gn {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, gp
}

func gpuText(nodes []gpuNode, pods []gpuPod) string {
	var sb strings.Builder
	if len(nodes) == 0 && len(pods) == 0 {
		return tagDim + "no GPU nodes and no pods asking for GPUs in this cluster" + tagReset + "\n"
	}
	sb.WriteString(heading(pad("NODE", 32), pad("RESOURCE", 16), pad("USED", 8), "BAR"))
	for _, n := range nodes {
		tag := statusTag("Running")
		switch {
		case n.Requested >= n.Allocatable:
			tag = statusTag("CrashLoopBackOff")
		case n.Requested*2 >= n.Allocatable:
			tag = statusTag("NoReady")
		}
		filled := int(n.Requested)
		bar := strings.Repeat("█", min(filled, 16)) + strings.Repeat("░", min(max(int(n.Allocatable)-filled, 0), 16))
		fmt.Fprintf(&sb, "%s%s%s %s %s%s%s\n", tag, pad(n.Name, 32), tagReset, pad(n.Kind, 16), pad(fmt.Sprintf("%d/%d", n.Requested, n.Allocatable), 8), tag, bar+tagReset)
	}
	sb.WriteString("\n" + heading(pad("POD", 48), pad("NODE", 24), pad("GPUS", 5), "STATUS"))
	for _, p := range pods {
		fmt.Fprintf(&sb, "%s%s%s %s %s %s%s%s\n", statusTag(p.Status), pad(p.Path, 48), tagReset, pad(firstNonEmpty(p.Node), 24), pad(fmt.Sprint(p.Count), 5), statusTag(p.Status), p.Status, tagReset)
	}

	return sb.String()
}

// gpuCmd shows GPU capacity and who is holding it in the active cluster.
func (a *App) gpuCmd() {
	ctxName := a.Config.ActiveContextName()
	a.textScreen("GPU", ctxName, func() string {
		cs, err := clientFor(ctxName)
		if err != nil {
			return err.Error()
		}
		ctx, cancel := context.WithTimeout(context.Background(), multiTimeout)
		defer cancel()
		nn, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return err.Error()
		}
		pp, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
		if err != nil {
			return err.Error()
		}

		return gpuText(gpuUsage(nn.Items, pp.Items))
	})
}
