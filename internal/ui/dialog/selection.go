// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package dialog

import (
	"github.com/derailed/tview"
	"github.com/rsmrtk/helmsman/internal/config"
	"github.com/rsmrtk/helmsman/internal/ui"
)

type SelectAction func(index int)

func ShowSelection(styles *config.Dialog, pages *ui.Pages, title string, options []string, action SelectAction) {
	list := tview.NewList()
	list.ShowSecondaryText(false)
	list.SetSelectedTextColor(styles.ButtonFocusFgColor.Color())
	list.SetSelectedBackgroundColor(styles.ButtonFocusBgColor.Color())

	for _, option := range options {
		list.AddItem(option, "", 0, nil)
	}

	modal := ui.NewModalList("<"+title+">", list)
	modal.SetDoneFunc(func(i int, _ string) {
		dismiss(pages)
		action(i)
	})

	pages.AddPage(dialogKey, modal, false, false)
	pages.ShowPage(dialogKey)
}
