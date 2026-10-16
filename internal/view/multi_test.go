// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDiffWorkloads(t *testing.T) {
	a := map[string]workload{
		"deploy/ns/web":  {Key: "deploy/ns/web", Images: []string{"web:1"}, Replicas: "2/2"},
		"deploy/ns/api":  {Key: "deploy/ns/api", Images: []string{"api:1"}, Replicas: "1/1"},
		"deploy/ns/solo": {Key: "deploy/ns/solo", Images: []string{"s:1"}, Replicas: "1/1"},
	}
	b := map[string]workload{
		"deploy/ns/web":  {Key: "deploy/ns/web", Images: []string{"web:2"}, Replicas: "2/2"},
		"deploy/ns/api":  {Key: "deploy/ns/api", Images: []string{"api:1"}, Replicas: "1/1"},
		"deploy/ns/only": {Key: "deploy/ns/only", Images: []string{"o:1"}, Replicas: "1/1"},
	}
	got := map[string]string{}
	for _, r := range diffWorkloads(a, b) {
		got[r.Key] = r.Kind
	}
	assert.Equal(t, map[string]string{
		"deploy/ns/web": "differs", "deploy/ns/api": "same",
		"deploy/ns/solo": "only-a", "deploy/ns/only": "only-b",
	}, got)

	rows := diffWorkloads(a, b)
	assert.Equal(t, "differs", rows[0].Kind, "differences come first")
	assert.Equal(t, "same", rows[len(rows)-1].Kind, "identical rows go last")
	assert.Contains(t, diffText("a", "b", "ns", rows), "1 identical, 3 to look at")
	assert.Contains(t, diffText("a", "b", "ns", diffWorkloads(a, a)), "no differences")
}

func TestSummarizePods(t *testing.T) {
	pods := []v1.Pod{
		{Status: v1.PodStatus{Phase: v1.PodRunning}},
		{Status: v1.PodStatus{Phase: v1.PodPending}},
		{Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{{State: v1.ContainerState{Waiting: &v1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}}}}},
	}
	total, problems, kinds := summarizePods(pods)
	assert.Equal(t, 3, total)
	assert.Equal(t, 2, problems)
	assert.Equal(t, map[string]int{"Pending": 1, "CrashLoopBackOff": 1}, kinds)
}

func TestSummarizeNodes(t *testing.T) {
	nodes := []v1.Node{
		{Status: v1.NodeStatus{Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}}}},
		{Status: v1.NodeStatus{Conditions: []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionFalse}}}},
	}
	total, ready := summarizeNodes(nodes)
	assert.Equal(t, 2, total)
	assert.Equal(t, 1, ready)
}

func TestGPUUsage(t *testing.T) {
	gpu := func(n string) v1.ResourceList { return v1.ResourceList{"nvidia.com/gpu": resource.MustParse(n)} }
	nodes := []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "gpu-1"}, Status: v1.NodeStatus{Allocatable: gpu("4")}},
		{ObjectMeta: metav1.ObjectMeta{Name: "cpu-1"}},
	}
	pods := []v1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "train", Namespace: "ml"}, Spec: v1.PodSpec{NodeName: "gpu-1", Containers: []v1.Container{{Resources: v1.ResourceRequirements{Requests: gpu("2")}}}}, Status: v1.PodStatus{Phase: v1.PodRunning}},
		{ObjectMeta: metav1.ObjectMeta{Name: "done", Namespace: "ml"}, Spec: v1.PodSpec{NodeName: "gpu-1", Containers: []v1.Container{{Resources: v1.ResourceRequirements{Requests: gpu("2")}}}}, Status: v1.PodStatus{Phase: v1.PodSucceeded}},
		{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ml"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
	}
	gn, gp := gpuUsage(nodes, pods)
	assert.Len(t, gn, 1)
	assert.Equal(t, int64(2), gn[0].Requested, "finished pods hold no GPU")
	assert.Equal(t, int64(4), gn[0].Allocatable)
	assert.Len(t, gp, 1)
	assert.Equal(t, "ml/train", gp[0].Path)
	assert.Contains(t, gpuText(nil, nil), "no GPU nodes")
}
