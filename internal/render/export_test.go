// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package render

// ResetTrends forgets every recorded CPU sample; tests call it so renders do
// not leak sparklines into each other.
func ResetTrends() {
	podTrends.mx.Lock()
	defer podTrends.mx.Unlock()
	podTrends.entries = map[string]*trendEntry{}
}
