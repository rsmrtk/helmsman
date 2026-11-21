// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package view

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/derailed/tcell/v2"
	"github.com/rsmrtk/helmsman/internal"
	"github.com/rsmrtk/helmsman/internal/client"
	"github.com/rsmrtk/helmsman/internal/dao"
	"github.com/rsmrtk/helmsman/internal/model1"
	"github.com/rsmrtk/helmsman/internal/render"
	"github.com/rsmrtk/helmsman/internal/slogs"
	"github.com/rsmrtk/helmsman/internal/ui"
	"github.com/rsmrtk/helmsman/internal/ui/dialog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// nodeAll is the synthetic first row of the nodes list: it selects every node at once, so Enter (or `p`) opens the pods
// of the whole cluster instead of those of one machine. It is not a real node and no node action applies to it.
const nodeAll = render.AllRowID

// Node represents a node view.
type Node struct {
	ResourceViewer
}

// NewNode returns a new node view.
func NewNode(gvr *client.GVR) ResourceViewer {
	n := Node{
		ResourceViewer: NewBrowser(gvr),
	}
	n.AddBindKeysFn(n.bindKeys)
	n.GetTable().SetDecorateFn(n.decorate)
	n.GetTable().SetEnterFn(n.nextStep)
	n.SetContextFn(n.nodeContext)

	return &n
}

// decorate prepends the synthetic "all" row (see addAllNodesRow).
func (n *Node) decorate(td *model1.TableData) {
	if n.App().Conn() == nil {
		return
	}
	addAllNodesRow(td)
}

// addAllNodesRow adds the synthetic "all" row. Its PODS cell carries the cluster-wide total so the row reads like the
// others; every other cell stays blank because CPU/MEM etc. are per-node figures. Idempotent: refreshes re-run it.
func addAllNodesRow(td *model1.TableData) {
	if td.RowCount() == 0 {
		return
	}
	if _, ok := td.FindRow(nodeAll); ok {
		return
	}
	fields := make(model1.Fields, td.HeaderCount())
	fields[0] = nodeAll
	if idx, ok := td.IndexOfHeader("PODS"); ok {
		total := 0
		td.RowsRange(func(_ int, re model1.RowEvent) bool {
			if c, err := strconv.Atoi(re.Row.Fields[idx]); err == nil {
				total += c
			}
			return true
		})
		fields[idx] = strconv.Itoa(total)
	}
	td.AddRow(model1.RowEvent{
		Kind: model1.EventUnchanged,
		Row:  model1.Row{ID: nodeAll, Fields: fields},
	})
}

// rejectAll flashes a hint and returns true when the synthetic "all" row is among the selection: cordon, uncordon and
// drain make sense for a real machine only.
func (n *Node) rejectAll(sels []string) bool {
	for _, s := range sels {
		if s == nodeAll {
			n.App().Flash().Warn("`all` is not a node: pick a real node for this action")
			return true
		}
	}

	return false
}

func (n *Node) nodeContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, internal.KeyPodCounting, !n.App().Config.K9s.DisablePodCounting)
}

func (n *Node) bindDangerousKeys(aa *ui.KeyActions) {
	aa.Bulk(ui.KeyMap{
		ui.KeyC: ui.NewKeyActionWithOpts(
			"Cordon",
			n.toggleCordonCmd(true),
			ui.ActionOpts{
				Visible:   true,
				Dangerous: true,
			},
		),
		ui.KeyU: ui.NewKeyActionWithOpts(
			"Uncordon",
			n.toggleCordonCmd(false),
			ui.ActionOpts{
				Visible:   true,
				Dangerous: true,
			},
		),
		ui.KeyR: ui.NewKeyActionWithOpts(
			"Drain",
			n.drainCmd,
			ui.ActionOpts{
				Visible:   true,
				Dangerous: true,
			},
		),
	})
	ct, err := n.App().Config.K9s.ActiveContext()
	if err != nil {
		slog.Error("No active context located", slogs.Error, err)
		return
	}
	if ct.FeatureGates.NodeShell && n.App().Config.K9s.ShellPod != nil {
		aa.Add(ui.KeyS, ui.NewKeyAction("Shell", n.sshCmd, true))
	}
}

func (n *Node) bindKeys(aa *ui.KeyActions) {
	if !n.App().Config.IsReadOnly() {
		n.bindDangerousKeys(aa)
	}

	aa.Bulk(ui.KeyMap{
		ui.KeyY: ui.NewKeyAction(yamlAction, n.yamlCmd, true),
		ui.KeyP: ui.NewKeyAction("Pods", n.podsCmd, true),
	})
}

// nextStep continues the cluster -> nodes -> namespaces -> pods drill-down. Enter used to jump to the pods of the
// selected node; that is still one key away (`p`), so nothing is lost, but the default descent is now the namespaces
// list, which keeps the flow the same whichever node is highlighted.
func (n *Node) nextStep(a *App, _ ui.Tabular, _ *client.GVR, _ string) {
	a.gotoResource(client.NsGVR.String(), "", false, true)
}

// podsCmd shows the pods scheduled on the highlighted node (what Enter did before the nodes step was added).
func (n *Node) podsCmd(evt *tcell.EventKey) *tcell.EventKey {
	path := n.GetTable().GetSelectedItem()
	if path == "" {
		return evt
	}
	n.showPods(n.App(), nil, nil, path)

	return nil
}

func (n *Node) showPods(a *App, _ ui.Tabular, _ *client.GVR, path string) {
	if path == nodeAll {
		// every node: no field selector, so the pods of all nodes (and all namespaces) are listed.
		showPods(a, client.NamespaceAll, nil, "")
		return
	}
	showPods(a, n.GetTable().GetSelectedItem(), nil, "spec.nodeName="+path)
}

func (n *Node) drainCmd(evt *tcell.EventKey) *tcell.EventKey {
	sels := n.GetTable().GetSelectedItems()
	if len(sels) == 0 {
		return evt
	}
	if n.rejectAll(sels) {
		return nil
	}

	opts := dao.DrainOptions{
		GracePeriodSeconds: -1,
		Timeout:            5 * time.Second,
	}
	ShowDrain(n, sels, opts, drainNode)

	return nil
}

func drainNode(v ResourceViewer, sels []string, opts dao.DrainOptions) {
	res, err := dao.AccessorFor(v.App().factory, v.GVR())
	if err != nil {
		v.App().Flash().Err(err)
		return
	}
	m, ok := res.(dao.NodeMaintainer)
	if !ok {
		v.App().Flash().Err(fmt.Errorf("expecting a maintainer for %q", v.GVR()))
		return
	}

	v.Stop()
	defer v.Start()
	{
		d := NewDetails(v.App(), "Drain Progress", "nodes", contentYAML, true)
		if err := v.App().inject(d, false); err != nil {
			v.App().Flash().Err(err)
		}
		for _, sel := range sels {
			if err := m.Drain(sel, opts, d.GetWriter()); err != nil {
				v.App().Flash().Err(err)
			}
		}
		v.Refresh()
	}
}

func (n *Node) toggleCordonCmd(cordon bool) func(evt *tcell.EventKey) *tcell.EventKey {
	return func(evt *tcell.EventKey) *tcell.EventKey {
		sels := n.GetTable().GetSelectedItems()
		if len(sels) == 0 {
			return evt
		}
		if n.rejectAll(sels) {
			return nil
		}

		title, msg := "Confirm ", ""
		if cordon {
			title, msg = title+"Cordon", "Cordon "
		} else {
			title, msg = title+"Uncordon", "Uncordon "
		}
		if len(sels) == 1 {
			msg += sels[0] + "?"
		} else {
			msg += fmt.Sprintf("(%d) marked %s?", len(sels), n.GVR().R())
		}
		d := n.App().Styles.Dialog()
		dialog.ShowConfirm(&d, n.App().Content.Pages, title, msg, func() {
			res, err := dao.AccessorFor(n.App().factory, n.GVR())
			if err != nil {
				n.App().Flash().Err(err)
				return
			}
			m, ok := res.(dao.NodeMaintainer)
			if !ok {
				n.App().Flash().Err(fmt.Errorf("expecting a maintainer for %q", n.GVR()))
				return
			}
			for _, s := range sels {
				if err := m.ToggleCordon(s, cordon); err != nil {
					n.App().Flash().Err(err)
				}
			}
			n.Refresh()
		}, func() {})

		return nil
	}
}

func (n *Node) sshCmd(evt *tcell.EventKey) *tcell.EventKey {
	path := n.GetTable().GetSelectedItem()
	if path == "" {
		return evt
	}

	n.Stop()
	defer n.Start()
	_, node := client.Namespaced(path)
	launchNodeShell(n, n.App(), node)

	return nil
}

func (n *Node) yamlCmd(evt *tcell.EventKey) *tcell.EventKey {
	path := n.GetTable().GetSelectedItem()
	if path == "" {
		return evt
	}

	n.Stop()
	defer n.Start()
	ctx, cancel := context.WithTimeout(context.Background(), n.App().Conn().Config().CallTimeout())
	defer cancel()

	sel := n.GetTable().GetSelectedItem()
	gvr := n.GVR().GVR()
	dial, err := n.App().factory.Client().DynDial()
	if err != nil {
		n.App().Flash().Err(err)
		return nil
	}
	o, err := dial.Resource(gvr).Get(ctx, sel, metav1.GetOptions{})
	if err != nil {
		n.App().Flash().Errf("Unable to get resource %q -- %s", n.GVR(), err)
		return nil
	}

	raw, err := dao.ToYAML(o, false)
	if err != nil {
		n.App().Flash().Errf("Unable to marshal resource %s", err)
		return nil
	}

	details := NewDetails(n.App(), yamlAction, sel, contentYAML, true).Update(raw)
	if err := n.App().inject(details, false); err != nil {
		n.App().Flash().Err(err)
	}

	return nil
}
