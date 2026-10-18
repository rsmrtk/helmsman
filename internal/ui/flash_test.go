// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package ui_test

import (
	"testing"

	"github.com/rsmrtk/helmsman/internal/config/mock"
	"github.com/rsmrtk/helmsman/internal/model"
	"github.com/rsmrtk/helmsman/internal/ui"
	"github.com/stretchr/testify/assert"
)

func TestFlash(t *testing.T) {
	uu := map[string]struct {
		l    model.FlashLevel
		i, e string
	}{
		"info": {l: model.FlashInfo, i: "hello", e: "⎈ hello\n"},
		"warn": {l: model.FlashWarn, i: "hello", e: "▲ hello\n"},
		"err":  {l: model.FlashErr, i: "hello", e: "✖ hello\n"},
	}

	a := ui.NewApp(mock.NewMockConfig(t), "test")
	f := ui.NewFlash(a)
	f.SetTestMode(true)

	for k := range uu {
		u := uu[k]
		t.Run(k, func(t *testing.T) {
			// test mode applies the message synchronously, so nothing reads the view
			// from another goroutine while the watcher writes to it
			f.SetMessage(model.LevelMessage{Level: u.l, Text: u.i})
			assert.Equal(t, u.e, f.GetText(false))
		})
	}
}
