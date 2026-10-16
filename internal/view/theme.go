// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"slices"

	"github.com/derailed/tcell/v2"
	"github.com/rsmrtk/helmsman/internal/config"
)

// themeCmd switches to the next skin and remembers it in the config.
func (a *App) themeCmd(*tcell.EventKey) *tcell.EventKey {
	names := config.SkinNames()
	cur := slices.Index(names, a.Config.K9s.UI.Skin)
	next := names[(cur+1)%len(names)] // an unknown current skin starts over at the default
	a.Config.K9s.UI.Skin = next
	a.ReloadStyles()

	label := next
	if label == "" {
		label = "helmsman"
	}
	a.Flash().Infof("Theme: %s", label)
	if err := a.Config.Save(true); err != nil {
		a.Flash().Err(err)
	}

	return nil
}
