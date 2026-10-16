// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package view

import (
	"github.com/derailed/tcell/v2"
	"github.com/rsmrtk/helmsman/internal/client"
	"github.com/rsmrtk/helmsman/internal/ui"
)

const faultsFilter = "Warning|Error"

// Event represents a command alias view.
type Event struct {
	ResourceViewer
}

// NewEvent returns a new alias view.
func NewEvent(gvr *client.GVR) ResourceViewer {
	e := Event{
		ResourceViewer: NewBrowser(gvr),
	}
	e.AddBindKeysFn(e.bindKeys)
	e.GetTable().SetSortCol("LAST SEEN", false)

	return &e
}

func (e *Event) bindKeys(aa *ui.KeyActions) {
	aa.Delete(tcell.KeyCtrlD, ui.KeyE, ui.KeyA)
	aa.Bulk(ui.KeyMap{
		tcell.KeyCtrlZ: ui.NewKeyAction("Toggle Faults", e.toggleFaults, false),
	})
}

func (e *Event) toggleFaults(*tcell.EventKey) *tcell.EventKey {
	b, ok := e.ResourceViewer.(*Browser)
	if !ok {
		return nil
	}
	filter := b.CmdBuff().GetText()
	if filter == faultsFilter {
		e.SetFilter("", true)
		e.App().Flash().Info("Showing all events")
	} else {
		e.SetFilter(faultsFilter, true)
		e.App().Flash().Info("Showing Warning and Error events only")
	}
	return nil
}
