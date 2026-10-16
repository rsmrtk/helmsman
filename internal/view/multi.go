// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Screens that look at more than one cluster, or at one in a way the resource
// browsers do not: :overview, :diff and :gpu. They render to text, so one small
// viewer serves all of them.

const multiTimeout = 6 * time.Second

// clientFor builds a clientset for a kubeconfig context without switching to it.
func clientFor(contextName string) (kubernetes.Interface, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules, &clientcmd.ConfigOverrides{CurrentContext: contextName},
	).ClientConfig()
	if err != nil {
		return nil, err
	}
	cfg.Timeout = multiTimeout

	return kubernetes.NewForConfig(cfg)
}

func contextNames() ([]string, error) {
	raw, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(raw.Contexts))
	for n := range raw.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)

	return names, nil
}

// forEachContext runs fn for every context at once and returns when all are done.
func forEachContext[T any](names []string, fn func(ctx context.Context, name string) T) []T {
	out := make([]T, len(names))
	var wg sync.WaitGroup
	for i, n := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), multiTimeout)
			defer cancel()
			out[i] = fn(ctx, n)
		}()
	}
	wg.Wait()

	return out
}

// textScreen opens a text viewer, fills it with "loading" and then with the
// result of build, which runs off the UI thread.
func (a *App) textScreen(title, subject string, build func() string) {
	d := NewDetails(a, title, subject, "", false)
	if err := a.inject(d, false); err != nil {
		a.Flash().Err(err)
		return
	}
	d.Update("[::d]loading…[-::-]")
	go func() {
		txt := build()
		a.QueueUpdateDraw(func() { d.Update(txt) })
	}()
}

// paletteTag is a tview colour tag for a pod status colour.
func paletteTag(status string) string {
	return statusTag(status)
}

func pad(s string, w int) string {
	if n := len([]rune(s)); n < w {
		return s + strings.Repeat(" ", w-n)
	}

	return s
}

func heading(cols ...string) string {
	return fmt.Sprintf("%s%s%s\n", tagHeading, strings.Join(cols, "  "), tagReset)
}
