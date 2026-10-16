// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package render

import (
	"strconv"
	"strings"

	"github.com/derailed/tcell/v2"
	"github.com/rsmrtk/helmsman/internal/model1"
)

// Row colours for resources other than pods. They reuse the pod status palette
// so one colour always means one thing: green is healthy, blue is finished,
// amber is waiting, raspberry is partial and red is broken.

func paletteColor(key string, fallback tcell.Color) tcell.Color {
	if c, ok := model1.StatusColor(key); ok {
		return c
	}

	return fallback
}

func fieldAt(h model1.Header, re *model1.RowEvent, col string) (string, bool) {
	i, ok := h.IndexOf(col, true)
	if !ok || i >= len(re.Row.Fields) {
		return "", false
	}

	return strings.TrimSpace(re.Row.Fields[i]), true
}

// readyColorer colours a workload by how many of its replicas are ready. The
// READY column is either "ready/desired" or a bare number with a DESIRED column.
func readyColorer() model1.ColorerFunc {
	return func(ns string, h model1.Header, re *model1.RowEvent) tcell.Color {
		c := model1.DefaultColorer(ns, h, re)
		if c == model1.ErrColor {
			return c
		}
		ready, ok := fieldAt(h, re, "READY")
		if !ok {
			return c
		}
		have, want, found := strings.Cut(ready, "/")
		if !found {
			d, ok := fieldAt(h, re, "DESIRED")
			if !ok {
				return c
			}
			have, want = ready, d
		}
		hv, err1 := strconv.Atoi(have)
		wv, err2 := strconv.Atoi(want)
		if err1 != nil || err2 != nil {
			return c
		}

		switch {
		case wv == 0:
			return paletteColor("completed", c) // scaled to zero: parked
		case hv >= wv:
			return paletteColor("running", c)
		case hv == 0:
			return paletteColor("pending", c)
		default:
			return paletteColor("noready", c)
		}
	}
}

// phaseColorer colours a row by the value of one column through the palette.
func phaseColorer(col string, m map[string]string) model1.ColorerFunc {
	return func(ns string, h model1.Header, re *model1.RowEvent) tcell.Color {
		c := model1.DefaultColorer(ns, h, re)
		if c == model1.ErrColor {
			return c
		}
		v, ok := fieldAt(h, re, col)
		if !ok {
			return c
		}
		if key, ok := m[strings.ToLower(v)]; ok {
			return paletteColor(key, c)
		}

		return c
	}
}

var (
	pvcPhases = map[string]string{"bound": "running", "pending": "pending", "lost": "crashloopbackoff", "terminating": "terminating"}
	pvPhases  = map[string]string{"bound": "running", "available": "completed", "pending": "pending", "released": "pending", "failed": "crashloopbackoff", "terminating": "terminating"}
	nsPhases  = map[string]string{"active": "running", "terminating": "terminating"}
)

// nodeColor colours a node by its STATUS, which is a comma list such as
// "Ready,SchedulingDisabled".
func nodeColor(status string, def tcell.Color) tcell.Color {
	switch {
	case strings.Contains(status, "NotReady"):
		return paletteColor("crashloopbackoff", def)
	case strings.Contains(status, "SchedulingDisabled"):
		return paletteColor("pending", def)
	case strings.Contains(status, "Ready"):
		return paletteColor("running", def)
	}

	return def
}

// crdWords maps the status words custom resources use onto palette entries.
var crdWords = map[string]string{
	"healthy": "running", "synced": "running", "ready": "running", "true": "running", "available": "running",
	"succeeded": "completed", "completed": "completed", "suspended": "completed",
	"progressing": "pending", "outofsync": "noready", "unknown": "unknown", "pending": "pending",
	"degraded": "crashloopbackoff", "missing": "crashloopbackoff", "failed": "crashloopbackoff",
	"false": "crashloopbackoff", "error": "error", "notready": "crashloopbackoff",
}

// crdStatusColumns are looked at in order; the first one with a known value wins,
// the worst-looking column deciding when several disagree.
var crdStatusColumns = []string{"HEALTH STATUS", "SYNC STATUS", "READY", "STATUS", "PHASE", "STATE", "HEALTH", "SYNCED"}

func crdColorer(ns string, h model1.Header, re *model1.RowEvent) tcell.Color {
	c := model1.DefaultColorer(ns, h, re)
	if c == model1.ErrColor {
		return c
	}
	worst, rank := "", -1
	order := map[string]int{"running": 0, "completed": 1, "pending": 2, "noready": 3, "unknown": 3, "error": 4, "crashloopbackoff": 5}
	for _, col := range crdStatusColumns {
		v, ok := fieldAt(h, re, col)
		if !ok {
			continue
		}
		key, ok := crdWords[strings.ToLower(v)]
		if !ok {
			continue
		}
		if r := order[key]; r > rank {
			worst, rank = key, r
		}
	}
	if worst == "" {
		return c
	}

	return paletteColor(worst, c)
}
