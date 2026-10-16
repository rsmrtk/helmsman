// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package model1

import (
	"strings"

	"github.com/derailed/tcell/v2"
)

// defaultStatusColors gives every pod status its own colour. Healthy pods are
// deliberately dim so that anything which needs attention stands out.
var defaultStatusColors = map[string]string{
	"running":                    "#47ba94", // healthy: green
	"completed":                  "#4999d6", // finished: blue
	"succeeded":                  "#4999d6",
	"noready":                    "#bf5677", // running, but not every container ready: raspberry
	"pending":                    "#f2c14e",
	"schedulinggated":            "#7aa2f7",
	"containercreating":          "#e5a050", // orange
	"podinitializing":            "#4fd6be", // cyan
	"initialized":                "#7ee0c3", // mint
	"init":                       "#4fd6be", // Init:0/2 and friends
	"terminating":                "#b4a0e5", // purple
	"crashloopbackoff":           "#ff5370", // red
	"oomkilled":                  "#ff2e63", // crimson
	"error":                      "#f07178", // coral
	"failed":                     "#f07178",
	"exitcode":                   "#f07178",
	"signal":                     "#f07178",
	"imagepullbackoff":           "#ff79c6", // magenta
	"errimagepull":               "#ff79c6",
	"invalidimagename":           "#ff79c6",
	"createcontainerconfigerror": "#ff9e64",
	"createcontainererror":       "#ff9e64",
	"evicted":                    "#c3733d", // rust
	"deadlineexceeded":           "#d98c8c",
	"unknown":                    "#9aa0c0", // lost track of it: grey-lavender
	"containerstatusunknown":     "#9aa0c0",
	"nodelost":                   "#9aa0c0",
}

// statusColors holds the active palette; the skin can override single entries.
var statusColors = buildStatusColors(nil)

func buildStatusColors(over map[string]string) map[string]tcell.Color {
	out := make(map[string]tcell.Color, len(defaultStatusColors)+len(over))
	for k, v := range defaultStatusColors {
		out[k] = tcell.GetColor(v)
	}
	for k, v := range over {
		out[strings.ToLower(k)] = tcell.GetColor(v)
	}
	return out
}

// SetStatusColors installs the palette with the skin's overrides applied.
func SetStatusColors(over map[string]string) {
	statusColors = buildStatusColors(over)
}

// StatusColor returns the colour for a pod status such as Running,
// CrashLoopBackOff, Init:0/2 or Init:Error. The second result is false when the
// status has no colour of its own.
func StatusColor(status string) (tcell.Color, bool) {
	s := strings.ToLower(strings.TrimSpace(status))
	if c, ok := statusColors[s]; ok {
		return c, true
	}
	if rest, ok := strings.CutPrefix(s, "init:"); ok {
		// Init:CrashLoopBackOff takes the colour of what went wrong; Init:1/3 is plain init.
		if c, ok := statusColors[rest]; ok {
			return c, true
		}
		if i := strings.IndexByte(rest, ':'); i > 0 {
			rest = rest[:i]
		}
		if c, ok := statusColors[rest]; ok {
			return c, true
		}
		return statusColors["init"], true
	}
	if i := strings.IndexByte(s, ':'); i > 0 {
		if c, ok := statusColors[s[:i]]; ok { // ExitCode:1, Signal:9
			return c, true
		}
	}
	return 0, false
}

// failureStatuses are the statuses that mean something is broken, as opposed
// to merely not finished; their rows are drawn bold so they stand out.
var failureStatuses = map[string]struct{}{
	"crashloopbackoff": {}, "oomkilled": {}, "error": {}, "exitcode": {}, "signal": {},
	"imagepullbackoff": {}, "errimagepull": {}, "invalidimagename": {},
	"createcontainerconfigerror": {}, "createcontainererror": {}, "evicted": {},
	"deadlineexceeded": {}, "containerstatusunknown": {},
}

// IsFailure reports whether a pod status is a failure, including Init:<failure>.
func IsFailure(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	s = strings.TrimPrefix(s, "init:")
	if i := strings.IndexByte(s, ':'); i > 0 {
		s = s[:i]
	}
	if _, ok := failureStatuses[s]; ok {
		return true
	}
	// a trailing delta marker, e.g. "CrashLoopBackOffΔ"
	_, ok := failureStatuses[strings.TrimRight(s, "Δδ↑↓")]

	return ok
}
