// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package view

import (
	"github.com/derailed/tcell/v2"
	"github.com/rsmrtk/helmsman/internal/client"
	"github.com/rsmrtk/helmsman/internal/ui"
)

// PriorityClass presents a priority class viewer.
type PriorityClass struct {
	ResourceViewer
}

// NewPriorityClass returns a new viewer.
func NewPriorityClass(gvr *client.GVR) ResourceViewer {
	s := PriorityClass{
		ResourceViewer: NewBrowser(gvr),
	}
	s.AddBindKeysFn(s.bindKeys)

	return &s
}

func (s *PriorityClass) bindKeys(aa *ui.KeyActions) {
	aa.Add(ui.KeyU, ui.NewKeyAction("UsedBy", s.refCmd, true))
}

func (s *PriorityClass) refCmd(evt *tcell.EventKey) *tcell.EventKey {
	return scanRefs(evt, s.App(), s.GetTable(), client.PcGVR)
}
