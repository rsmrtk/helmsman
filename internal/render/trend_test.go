// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package render

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTrendLabel(t *testing.T) {
	assert.Equal(t, "", trendLabel(nil))
	assert.Equal(t, "", trendLabel([]int64{5}), "one reading is not a trend")
	assert.Equal(t, "→", trendLabel([]int64{100, 100, 100}), "steady")
	assert.Equal(t, "→", trendLabel([]int64{100, 102, 101}), "wobble is ignored")
	assert.Equal(t, "→", trendLabel([]int64{0, 0, 3}), "a few millicores of idle noise")
	assert.Equal(t, "↑ +50m", trendLabel([]int64{100, 100, 150}))
	assert.Equal(t, "↓ -60m", trendLabel([]int64{100, 100, 40}))
}

func TestTrendsWindowAndPrune(t *testing.T) {
	now := time.Unix(0, 0)
	tr := &trends{entries: map[string]*trendEntry{}, now: func() time.Time { return now }}

	var out string
	for i := range trendWindow + 5 {
		out = tr.record("ns/a", int64(i))
	}
	assert.Equal(t, "↑ +5m", out, "a steady climb shows as rising")
	assert.LessOrEqual(t, len(tr.entries["ns/a"].samples), trendWindow, "only the last readings are kept")
	assert.Equal(t, "", tr.record("ns/b", 1), "a pod seen once has no trend yet")

	for i := range 300 {
		tr.record("old/"+string(rune('a'+i%26))+string(rune('a'+i/26)), 1)
	}
	now = now.Add(trendStale + time.Minute)
	tr.record("ns/fresh", 1)
	assert.Contains(t, tr.entries, "ns/fresh")
	assert.NotContains(t, tr.entries, "ns/a", "stale pods are forgotten")
}
