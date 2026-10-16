// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package render_test

import (
	"testing"

	"github.com/derailed/tcell/v2"
	"github.com/rsmrtk/helmsman/internal/model1"
	"github.com/rsmrtk/helmsman/internal/render"
	"github.com/stretchr/testify/assert"
)

func TestDeploymentReadyColors(t *testing.T) {
	h := model1.Header{
		model1.HeaderColumn{Name: "NAMESPACE"},
		model1.HeaderColumn{Name: "NAME"},
		model1.HeaderColumn{Name: "READY"},
	}
	col := render.Deployment{}.ColorerFunc()
	color := func(ready string) tcell.Color {
		re := model1.RowEvent{Kind: model1.EventUnchanged, Row: model1.Row{Fields: model1.Fields{"ns", "web", ready}}}
		return col("ns", h, &re)
	}
	pal := func(s string) tcell.Color { c, _ := model1.StatusColor(s); return c }

	assert.Equal(t, pal("Running"), color("3/3"))
	assert.Equal(t, pal("NoReady"), color("1/3"))
	assert.Equal(t, pal("Pending"), color("0/2"))
	assert.Equal(t, pal("Completed"), color("0/0"))
}

func TestPVCPhaseColors(t *testing.T) {
	h := model1.Header{model1.HeaderColumn{Name: "NAME"}, model1.HeaderColumn{Name: "STATUS"}}
	col := render.PersistentVolumeClaim{}.ColorerFunc()
	color := func(s string) tcell.Color {
		re := model1.RowEvent{Kind: model1.EventUnchanged, Row: model1.Row{Fields: model1.Fields{"data", s}}}
		return col("ns", h, &re)
	}
	pal := func(s string) tcell.Color { c, _ := model1.StatusColor(s); return c }

	assert.Equal(t, pal("Running"), color("Bound"))
	assert.Equal(t, pal("Pending"), color("Pending"))
	assert.Equal(t, pal("CrashLoopBackOff"), color("Lost"))
}

func TestCRDColorer(t *testing.T) {
	h := model1.Header{
		model1.HeaderColumn{Name: "NAME"},
		model1.HeaderColumn{Name: "SYNC STATUS"},
		model1.HeaderColumn{Name: "HEALTH STATUS"},
	}
	col := (&render.Table{}).ColorerFunc()
	color := func(sync, health string) tcell.Color {
		re := model1.RowEvent{Kind: model1.EventUnchanged, Row: model1.Row{Fields: model1.Fields{"app", sync, health}}}
		return col("ns", h, &re)
	}
	pal := func(s string) tcell.Color { c, _ := model1.StatusColor(s); return c }

	assert.Equal(t, pal("Running"), color("Synced", "Healthy"))
	assert.Equal(t, pal("CrashLoopBackOff"), color("Synced", "Degraded"), "the worst column wins")
	assert.Equal(t, pal("NoReady"), color("OutOfSync", "Healthy"))
	assert.Equal(t, pal("Pending"), color("Synced", "Progressing"))
}
