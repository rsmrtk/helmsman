// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func crashPod() *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
		Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "c"}}},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
			ContainerStatuses: []v1.ContainerStatus{{
				Name:         "c",
				RestartCount: 4,
				State:        v1.ContainerState{Waiting: &v1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				LastTerminationState: v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137},
				},
			}},
		},
	}
}

func TestPodPhase(t *testing.T) {
	assert.Equal(t, "CrashLoopBackOff", podPhase(crashPod()))

	p := &v1.Pod{Status: v1.PodStatus{Phase: v1.PodPending}}
	assert.Equal(t, "Pending", podPhase(p))

	now := metav1.Now()
	p = crashPod()
	p.DeletionTimestamp = &now
	assert.Equal(t, "Terminating", podPhase(p))
}

func TestDiagnoseText(t *testing.T) {
	events := []v1.Event{{Type: v1.EventTypeWarning, Reason: "BackOff", Message: "Back-off restarting [c]"}}
	txt := diagnoseText(crashPod(), events, map[string]string{"c": "panic: boom\n"})

	for _, want := range []string{
		"prod/api", "CrashLoopBackOff", "Findings", "last terminated: OOMKilled, exit code 137",
		"sets no requests or limits", "Containers", "Events", "BackOff", "Previous logs: c", "panic: boom",
	} {
		assert.Contains(t, txt, want)
	}
	assert.NotContains(t, txt, "Back-off restarting [c]", "square brackets are escaped for the tag parser")
}

func TestDiagnoseHealthyPod(t *testing.T) {
	p := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "ok", Namespace: "prod"},
		Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "c", Resources: v1.ResourceRequirements{Requests: v1.ResourceList{v1.ResourceCPU: {}}}}}},
		Status:     v1.PodStatus{Phase: v1.PodRunning, ContainerStatuses: []v1.ContainerStatus{{Name: "c", Ready: true, State: v1.ContainerState{Running: &v1.ContainerStateRunning{}}}}},
	}
	assert.Contains(t, diagnoseText(p, nil, nil), "nothing wrong found")
}

func TestFindingAdviceIsSeparate(t *testing.T) {
	ff := findings(crashPod(), nil)
	var waiting finding
	for _, f := range ff {
		if f.severity == "CrashLoopBackOff" {
			waiting = f
		}
	}
	assert.Contains(t, waiting.text, "waiting: CrashLoopBackOff")
	assert.Contains(t, waiting.advice, "previous logs", "advice survives however long the message is")
}
