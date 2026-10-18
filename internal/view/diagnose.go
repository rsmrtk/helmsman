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
	"github.com/rsmrtk/helmsman/internal/model"
	"github.com/rsmrtk/helmsman/internal/ui"
	"github.com/rsmrtk/helmsman/internal/ui/dialog"
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
	if !p.App().Config.IsReadOnly() {
		d.Actions().Add(ui.KeyR, ui.NewKeyAction("Restart Pod", p.restartPodCmd(path), true))
	}
	if err := p.App().inject(d, false); err != nil {
		p.App().Flash().Err(err)
		return nil
	}
	d.Update(diagnoseText(pod, events, prev))
	p.refreshDiagnosis(d, path)

	return nil
}

// diagnoseRefresh is how often an open diagnosis re-reads the pod.
const diagnoseRefresh = 3 * time.Second

// refreshDiagnosis keeps an open diagnosis current while the pod recovers or
// keeps failing. It stops as soon as the user leaves the view.
func (p *Pod) refreshDiagnosis(d *Details, path string) {
	app := p.App()
	ns, name := client.Namespaced(path)
	go func() {
		t := time.NewTicker(diagnoseRefresh)
		defer t.Stop()
		for range t.C {
			open := make(chan bool, 1)
			app.QueueUpdateDraw(func() { open <- app.Content.Top() == model.Component(d) })
			if !<-open {
				return
			}
			pod, err := fetchPod(app.factory, path)
			if err != nil {
				app.QueueUpdateDraw(func() { d.Update(tagDim + "pod is gone: " + err.Error() + tagReset) })
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), diagTimeout)
			events, prev := p.diagnosisData(ctx, pod, ns, name)
			cancel()
			text := diagnoseText(pod, events, prev)
			app.QueueUpdateDraw(func() { d.Update(text) })
		}
	}()
}

// restartPodCmd deletes the pod after confirmation; its controller recreates it.
func (p *Pod) restartPodCmd(path string) func(*tcell.EventKey) *tcell.EventKey {
	return func(*tcell.EventKey) *tcell.EventKey {
		app := p.App()
		ns, name := client.Namespaced(path)
		styles := app.Styles.Dialog()
		dialog.ShowConfirm(&styles, app.Content.Pages, "Restart",
			fmt.Sprintf("Delete pod %s/%s so its controller recreates it?", ns, name),
			func() {
				cs, err := app.factory.Client().Dial()
				if err == nil {
					err = cs.CoreV1().Pods(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
				}
				if err != nil {
					app.Flash().Err(err)
					return
				}
				app.Flash().Infof("Deleted pod %s; waiting for its replacement", name)
			},
			func() {},
		)

		return nil
	}
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
