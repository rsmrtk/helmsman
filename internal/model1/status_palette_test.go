// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package model1_test

import (
	"testing"

	"github.com/derailed/tcell/v2"
	"github.com/rsmrtk/helmsman/internal/model1"
	"github.com/stretchr/testify/assert"
)

func TestStatusColor(t *testing.T) {
	hex := func(s string) tcell.Color { return tcell.GetColor(s) }

	uu := map[string]struct {
		status string
		e      tcell.Color
		ok     bool
	}{
		"running":       {"Running", hex("#47ba94"), true},
		"completed":     {"Completed", hex("#4999d6"), true},
		"crash":         {"CrashLoopBackOff", hex("#ff5370"), true},
		"case":          {"  crashloopbackoff ", hex("#ff5370"), true},
		"init-count":    {"Init:0/2", hex("#4fd6be"), true},
		"init-failure":  {"Init:CrashLoopBackOff", hex("#ff5370"), true},
		"init-exitcode": {"Init:ExitCode:1", hex("#f07178"), true},
		"exitcode":      {"ExitCode:137", hex("#f07178"), true},
		"signal":        {"Signal:9", hex("#f07178"), true},
		"unknown-word":  {"Whatever", 0, false},
	}
	for k, u := range uu {
		t.Run(k, func(t *testing.T) {
			c, ok := model1.StatusColor(u.status)
			assert.Equal(t, u.ok, ok)
			assert.Equal(t, u.e, c)
		})
	}
}

func TestSetStatusColorsOverrides(t *testing.T) {
	t.Cleanup(func() { model1.SetStatusColors(nil) })
	model1.SetStatusColors(map[string]string{"Running": "#010203"})

	c, ok := model1.StatusColor("Running")
	assert.True(t, ok)
	assert.Equal(t, tcell.GetColor("#010203"), c)
	c, _ = model1.StatusColor("CrashLoopBackOff")
	assert.Equal(t, tcell.GetColor("#ff5370"), c, "other statuses keep their colour")
}

func TestIsFailure(t *testing.T) {
	for s, e := range map[string]bool{
		"Running": false, "Completed": false, "Pending": false, "Init:0/1": false,
		"CrashLoopBackOff": true, "OOMKilled": true, "Error": true, "ErrImagePull": true,
		"Init:CrashLoopBackOff": true, "ExitCode:1": true, "CrashLoopBackOffΔ": true,
	} {
		assert.Equal(t, e, model1.IsFailure(s), s)
	}
}
