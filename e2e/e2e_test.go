// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

//go:build e2e

// Package e2e checks helmsman against a real cluster that runs demo/workloads.yaml.
// Run it with: E2E_CONTEXT=kind-hm-demo go test -tags e2e ./e2e/...
package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rsmrtk/helmsman/internal/model1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func podStatuses(t *testing.T) map[string]string {
	t.Helper()
	ctxName := os.Getenv("E2E_CONTEXT")
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{CurrentContext: ctxName}).ClientConfig()
	require.NoError(t, err)
	cs, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pods, err := cs.CoreV1().Pods("demo").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	out := map[string]string{}
	for _, p := range pods.Items {
		out[p.Name] = string(p.Status.Phase)
		for _, c := range p.Status.ContainerStatuses {
			if w := c.State.Waiting; w != nil && w.Reason != "" {
				out[p.Name] = w.Reason
			}
			if tm := c.State.Terminated; tm != nil && tm.Reason != "" && tm.Reason != "Completed" {
				out[p.Name] = tm.Reason
			}
		}
	}

	return out
}

// Every state the demo cluster produces must have a colour of its own, and the
// failing ones must be recognised as failures.
func TestDemoStatusesAreColoured(t *testing.T) {
	if os.Getenv("E2E_CONTEXT") == "" {
		t.Skip("set E2E_CONTEXT to a context running demo/workloads.yaml")
	}
	st := podStatuses(t)
	require.NotEmpty(t, st, "demo namespace has no pods: apply demo/workloads.yaml first")

	seen := map[string]bool{}
	for name, s := range st {
		_, ok := model1.StatusColor(s)
		assert.True(t, ok, "pod %s has status %q with no palette colour", name, s)
		seen[s] = true
	}
	for _, failing := range []string{"CrashLoopBackOff", "ImagePullBackOff"} {
		if seen[failing] {
			assert.True(t, model1.IsFailure(failing), failing)
		}
	}
	assert.True(t, seen["Running"], "demo should have healthy pods")
}
