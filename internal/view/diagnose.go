// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/derailed/tcell/v2"
	"github.com/rsmrtk/helmsman/internal/client"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const diagTimeout = 8 * time.Second

// diagnoseCmd explains why the selected pod is not healthy: findings with
// advice, container states, events and the tail of the previous logs.
func (p *Pod) diagnoseCmd(*tcell.EventKey) *tcell.EventKey {
	path := p.GetTable().GetSelectedItem()
	if path == "" {
		return nil
	}
	pod, err := fetchPod(p.App().factory, path)
	if err != nil {
		p.App().Flash().Err(err)
		return nil
	}
	ns, name := client.Namespaced(path)

	ctx, cancel := context.WithTimeout(context.Background(), diagTimeout)
	defer cancel()
	events, prev := p.diagnosisData(ctx, pod, ns, name)

	d := NewDetails(p.App(), "Diagnose", path, "", false)
	if c, ok := p.GetTable().RowColor(path); ok {
		d.SetAccent(c)
	}
	if err := p.App().inject(d, false); err != nil {
		p.App().Flash().Err(err)
		return nil
	}
	d.Update(diagnoseText(pod, events, prev))

	return nil
}

// diagnosisData fetches the pod's events and, for containers that have
// restarted, the tail of their previous logs. Failures only thin the report.
func (p *Pod) diagnosisData(ctx context.Context, pod *v1.Pod, ns, name string) (events []v1.Event, prev map[string]string) {
	prev = map[string]string{}
	cs, err := p.App().factory.Client().Dial()
	if err != nil {
		return nil, prev
	}
	if list, err := cs.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=Pod", name),
	}); err == nil {
		events = list.Items
	}
	tail := int64(diagTail)
	for _, st := range pod.Status.ContainerStatuses {
		if st.RestartCount == 0 && st.LastTerminationState.Terminated == nil {
			continue
		}
		rc, err := cs.CoreV1().Pods(ns).GetLogs(name, &v1.PodLogOptions{Container: st.Name, Previous: true, TailLines: &tail}).Stream(ctx)
		if err != nil {
			continue
		}
		if b, err := io.ReadAll(io.LimitReader(rc, 64<<10)); err == nil && len(b) > 0 {
			prev[st.Name] = string(b)
		}
		_ = rc.Close()
	}

	return events, prev
}
