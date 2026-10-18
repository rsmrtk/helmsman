// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package render

import (
	"fmt"
	"sync"
	"time"
)

// trendWindow is how many CPU samples a pod's sparkline shows.
const (
	trendWindow = 10
	trendStale  = 10 * time.Minute
)

type trendEntry struct {
	samples []int64
	seen    time.Time
}

// trends remembers the last CPU readings of every pod seen, so a table can show
// a trend without asking the metrics API for history it does not keep.
type trends struct {
	mx      sync.Mutex
	entries map[string]*trendEntry
	now     func() time.Time
}

var podTrends = &trends{entries: map[string]*trendEntry{}, now: time.Now}

// record stores a sample for id and returns where the pod's CPU is heading.
func (t *trends) record(id string, milli int64) string {
	t.mx.Lock()
	defer t.mx.Unlock()

	now := t.now()
	e, ok := t.entries[id]
	if !ok {
		e = &trendEntry{}
		t.entries[id] = e
	}
	e.samples = append(e.samples, milli)
	if len(e.samples) > trendWindow {
		e.samples = e.samples[len(e.samples)-trendWindow:]
	}
	e.seen = now
	t.prune(now)

	return trendLabel(e.samples)
}

// prune forgets pods that have not been seen for a while, so the map follows the cluster.
func (t *trends) prune(now time.Time) {
	if len(t.entries) < 256 {
		return
	}
	for id, e := range t.entries {
		if now.Sub(e.seen) > trendStale {
			delete(t.entries, id)
		}
	}
}

// trendLabel says where CPU is heading: an arrow and how many millicores the
// latest reading is above or below the average of the earlier ones. Plain text
// on purpose; block glyphs merge into solid bars in many terminal fonts.
func trendLabel(samples []int64) string {
	if len(samples) < 2 {
		return ""
	}
	last := samples[len(samples)-1]
	var sum int64
	for _, v := range samples[:len(samples)-1] {
		sum += v
	}
	avg := sum / int64(len(samples)-1)
	delta := last - avg

	// ignore wobble: under 5m, or under a tenth of what the pod normally uses
	if abs(delta) < max(5, avg/10) {
		return "→"
	}
	if delta > 0 {
		return fmt.Sprintf("↑ +%dm", delta)
	}

	return fmt.Sprintf("↓ %dm", delta)
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}

	return v
}
