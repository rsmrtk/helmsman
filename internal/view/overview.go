// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/rsmrtk/helmsman/internal/model1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// clusterSummary is one row of :overview.
type clusterSummary struct {
	Name         string
	Err          string
	Version      string
	Nodes        int
	NodesReady   int
	Pods         int
	Problems     int
	ProblemKinds map[string]int
}

func summarizeNodes(nn []v1.Node) (total, ready int) {
	for _, n := range nn {
		total++
		for _, c := range n.Status.Conditions {
			if c.Type == v1.NodeReady && c.Status == v1.ConditionTrue {
				ready++
			}
		}
	}

	return total, ready
}

// summarizePods counts pods and those in a failing or stuck state.
func summarizePods(pp []v1.Pod) (total, problems int, kinds map[string]int) {
	kinds = map[string]int{}
	for i := range pp {
		total++
		st := podPhase(&pp[i])
		bad := model1.IsFailure(st)
		switch st {
		case "Pending", "Unknown", "Terminating":
			bad = true
		}
		if bad {
			problems++
			kinds[st]++
		}
	}

	return total, problems, kinds
}

func fetchSummary(ctx context.Context, name string) clusterSummary {
	s := clusterSummary{Name: name}
	cs, err := clientFor(name)
	if err != nil {
		s.Err = err.Error()
		return s
	}
	if v, err := cs.Discovery().ServerVersion(); err == nil {
		s.Version = v.GitVersion
	} else {
		s.Err = shortErr(err)
		return s
	}
	if nn, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{}); err == nil {
		s.Nodes, s.NodesReady = summarizeNodes(nn.Items)
	}
	if pp, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{}); err == nil {
		s.Pods, s.Problems, s.ProblemKinds = summarizePods(pp.Items)
	}

	return s
}

func shortErr(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 && len(s) > 60 {
		s = s[i+2:]
	}
	if len(s) > 60 {
		s = s[:60] + "…"
	}

	return s
}

// overviewText renders the rows: unreachable clusters in red, clusters with
// problems in raspberry, healthy ones in green.
func overviewText(rows []clusterSummary, current string) string {
	var sb strings.Builder
	sb.WriteString(heading(pad("CLUSTER", 26), pad("VERSION", 14), pad("NODES", 7), pad("PODS", 6), pad("PROBLEMS", 9), "DETAIL"))
	for _, r := range rows {
		mark := " "
		if r.Name == current {
			mark = "*"
		}
		if r.Err != "" {
			fmt.Fprintf(&sb, "%s%s%s%s  %s%s\n", statusTag("CrashLoopBackOff"), mark, pad(r.Name, 25), tagReset, tagDim, "unreachable: "+r.Err)
			continue
		}
		tag := statusTag("Running")
		if r.Problems > 0 {
			tag = statusTag("NoReady")
		}
		if r.NodesReady < r.Nodes {
			tag = statusTag("CrashLoopBackOff")
		}
		detail := problemDetail(r.ProblemKinds)
		fmt.Fprintf(&sb, "%s%s%s%s  %-14s %-7s %-6d %-9d %s\n", tag, mark, pad(r.Name, 25), tagReset, r.Version,
			fmt.Sprintf("%d/%d", r.NodesReady, r.Nodes), r.Pods, r.Problems, detail)
	}
	fmt.Fprintf(&sb, "\n%s* is the active context. Enter a cluster with :ctx <name>.%s\n", tagDim, tagReset)

	return sb.String()
}

func problemDetail(kinds map[string]int) string {
	type kv struct {
		k string
		n int
	}
	var kk []kv
	for k, n := range kinds {
		kk = append(kk, kv{k, n})
	}
	sort.Slice(kk, func(i, j int) bool {
		if kk[i].n != kk[j].n {
			return kk[i].n > kk[j].n
		}
		return kk[i].k < kk[j].k
	})
	var parts []string
	for _, e := range kk {
		parts = append(parts, fmt.Sprintf("%s%s %d%s", statusTag(e.k), e.k, e.n, tagReset))
	}

	return strings.Join(parts, "  ")
}

// overviewCmd shows every kubeconfig context in one table.
func (a *App) overviewCmd() {
	current := a.Config.ActiveContextName()
	a.textScreen("Overview", "all clusters", func() string {
		names, err := contextNames()
		if err != nil {
			return "cannot read kubeconfig: " + err.Error()
		}
		rows := forEachContext(names, fetchSummary)

		return overviewText(rows, current)
	})
}
