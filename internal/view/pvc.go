// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package view

import (
	"github.com/derailed/tcell/v2"
	"github.com/rsmrtk/helmsman/internal/client"
	"github.com/rsmrtk/helmsman/internal/ui"
)

// PersistentVolumeClaim represents a PVC custom viewer.
type PersistentVolumeClaim struct {
	ResourceViewer
}

// NewPersistentVolumeClaim returns a new viewer.
func NewPersistentVolumeClaim(gvr *client.GVR) ResourceViewer {
	v := PersistentVolumeClaim{
		ResourceViewer: NewOwnerExtender(NewBrowser(gvr)),
	}
	v.AddBindKeysFn(v.bindKeys)

	return &v
}

func (p *PersistentVolumeClaim) bindKeys(aa *ui.KeyActions) {
	aa.Bulk(ui.KeyMap{
		ui.KeyU: ui.NewKeyAction("UsedBy", p.refCmd, true),
	})
}

func (p *PersistentVolumeClaim) refCmd(evt *tcell.EventKey) *tcell.EventKey {
	return scanRefs(evt, p.App(), p.GetTable(), client.PvcGVR)
}
