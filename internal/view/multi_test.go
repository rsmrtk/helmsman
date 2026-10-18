// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestFingerprintHidesValues(t *testing.T) {
	a, b := fingerprint("password", "ONE"), fingerprint("password", "TWO")
	assert.NotEqual(t, a, b, "different values give different fingerprints")
	assert.Equal(t, a, fingerprint("password", "ONE"))
	assert.NotContains(t, a, "ONE")
	assert.Len(t, a, 8)
	assert.NotEqual(t, fingerprint("a", "bc"), fingerprint("ab", "c"), "key/value boundaries matter")
}

func TestCollectWarnings(t *testing.T) {
	ev := func(typ, kind, name, reason string, count int32, at int64) v1.Event {
		return v1.Event{
			Type: typ, Reason: reason, Count: count, Message: reason + " msg",
			InvolvedObject: v1.ObjectReference{Kind: kind, Name: name, Namespace: "demo"},
			LastTimestamp:  metav1.NewTime(time.Unix(at, 0)),
		}
	}
	ww := collectWarnings([]v1.Event{
		ev("Normal", "Pod", "ok", "Started", 1, 10),
		ev("Warning", "Pod", "crash", "BackOff", 5, 20),
		ev("Warning", "Pod", "crash", "BackOff", 3, 30),
		ev("Warning", "Pod", "web", "Unhealthy", 9, 5),
		ev("Warning", "Node", "n1", "NodeNotReady", 0, 40),
	})
	require.Len(t, ww, 3, "Normal events are dropped and repeats merged")
	assert.Equal(t, "demo/pod/web", ww[0].Object, "the most repeated warning comes first")
	assert.Equal(t, int32(9), ww[0].Count)
	assert.Equal(t, "BackOff", ww[1].Reason)
	assert.Equal(t, int32(8), ww[1].Count, "repeats of the same object and reason are merged")
	assert.Equal(t, int32(1), ww[2].Count, "a zero count still counts as one")

	txt := warningsText(ww, time.Unix(100, 0))
	assert.Contains(t, txt, "3 distinct warnings")
	assert.Contains(t, warningsText(nil, time.Now()), "no warnings")
	assert.Equal(t, "40s", since(time.Unix(60, 0), time.Unix(100, 0)))
	assert.Equal(t, "3d", since(time.Unix(0, 0), time.Unix(3*86400, 0)))
}

func TestParseDrift(t *testing.T) {
	app := map[string]any{
		"metadata": map[string]any{"name": "web"},
		"status": map[string]any{
			"sync":           map[string]any{"status": "OutOfSync", "revision": "abcdef1234567890"},
			"health":         map[string]any{"status": "Degraded"},
			"operationState": map[string]any{"phase": "Failed"},
			"resources": []any{
				map[string]any{"kind": "Service", "name": "web", "namespace": "ns", "status": "Synced", "health": map[string]any{"status": "Healthy"}},
				map[string]any{"kind": "Deployment", "name": "web", "namespace": "ns", "status": "OutOfSync", "health": map[string]any{"status": "Degraded", "message": "0/2 [ready]"}},
				map[string]any{"kind": "ConfigMap", "name": "cfg", "namespace": "ns", "status": "OutOfSync"},
			},
			"conditions": []any{map[string]any{"type": "ComparisonError", "message": "bad manifest"}},
		},
	}
	r := parseDrift(app)
	assert.Equal(t, "web", r.Name)
	require.Len(t, r.Items, 3)
	assert.Equal(t, "ns/Deployment/web", r.Items[0].Ref, "degraded first")
	assert.Equal(t, "ns/ConfigMap/cfg", r.Items[1].Ref, "then out of sync")
	assert.Equal(t, "ns/Service/web", r.Items[2].Ref, "healthy last")

	txt := driftText(r)
	assert.Contains(t, txt, "2 of 3 resources need attention")
	assert.Contains(t, txt, "abcdef12")
	assert.Contains(t, txt, "ComparisonError: bad manifest")
	assert.Contains(t, txt, "0/2 [[ready]", "tags in messages are escaped")

	clean := parseDrift(map[string]any{"metadata": map[string]any{"name": "ok"}, "status": map[string]any{
		"sync":      map[string]any{"status": "Synced"},
		"resources": []any{map[string]any{"kind": "Service", "name": "s", "status": "Synced", "health": map[string]any{"status": "Healthy"}}},
	}})
	assert.Contains(t, driftText(clean), "no drift")
}
