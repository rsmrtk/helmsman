// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package dialog

import (
	"context"

	"github.com/derailed/tview"
	"github.com/rsmrtk/helmsman/internal/config"
	"github.com/rsmrtk/helmsman/internal/ui"
)

type promptAction func(ctx context.Context)

// ShowPrompt pops a prompt dialog.
func ShowPrompt(styles *config.Dialog, pages *ui.Pages, title, msg string, action promptAction, cancel cancelFunc) {
	f := tview.NewForm()
	f.SetItemPadding(0)
	f.SetButtonsAlign(tview.AlignCenter).
		SetButtonBackgroundColor(styles.ButtonBgColor.Color()).
		SetButtonTextColor(styles.ButtonFgColor.Color()).
		SetLabelColor(styles.LabelFgColor.Color()).
		SetFieldTextColor(styles.FieldFgColor.Color())

	ctx, cancelCtx := context.WithCancel(context.Background())

	f.AddButton("Cancel", func() {
		dismiss(pages)
		cancelCtx()
		cancel()
	})

	for i := range f.GetButtonCount() {
		b := f.GetButton(i)
		if b == nil {
			continue
		}
		b.SetBackgroundColorActivated(styles.ButtonFocusBgColor.Color())
		b.SetLabelColorActivated(styles.ButtonFocusFgColor.Color())
	}

	f.SetFocus(0)
	modal := tview.NewModalForm("<"+title+">", f)
	modal.SetText(msg)
	modal.SetTextColor(styles.FgColor.Color())
	modal.SetDoneFunc(func(int, string) {
		dismiss(pages)
		cancelCtx()
		cancel()
	})

	pages.AddPage(dialogKey, modal, false, false)
	pages.ShowPage(dialogKey)

	go func() {
		action(ctx)
		dismiss(pages)
	}()
}
