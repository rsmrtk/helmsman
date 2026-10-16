// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package dialog

import (
	"testing"

	"github.com/derailed/tview"
	"github.com/rsmrtk/helmsman/internal/config"
	"github.com/rsmrtk/helmsman/internal/ui"
	"github.com/stretchr/testify/assert"
)

func TestConfirmDialog(t *testing.T) {
	a := tview.NewApplication()
	p := ui.NewPages()
	a.SetRoot(p, false)
	ShowConfirm(new(config.Dialog), p, "Blee", "Yo", func() {}, func() {})

	d := p.GetPrimitive(dialogKey).(*tview.ModalForm)
	assert.NotNil(t, d)

	dismiss(p)
	assert.Nil(t, p.GetPrimitive(dialogKey))
}
