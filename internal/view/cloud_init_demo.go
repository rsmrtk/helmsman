// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"context"
	"io"
	"time"
)

// InitDemo plays the loading animation of every provider for a few seconds each, without touching any cloud. Real cloud
// CLIs answer in a second or two, so this is the way to see (and judge) the animations properly.
func InitDemo(ctx context.Context, w io.Writer, per time.Duration) error {
	ui := newInitUI(w, false)
	ui.begin()
	defer ui.end()
	ui.header(false)
	for _, prov := range []string{provAWS, provGCP, provAzure} {
		th := themeOf(prov)
		ui.stage(prov, func() {
			ui.setStatus("demo of the " + th.Title + " animation, no cloud is contacted…")
			select {
			case <-time.After(per):
			case <-ctx.Done():
			}
			ui.ok("demo", "animation finished")
			ui.blank()
		})
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	ui.printf("  %s\n\n", ui.paint(cDim, "That was `helmsman init demo`. The real commands: helmsman init aws | gcp | azure"))

	return nil
}
