// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package ui

import (
	"slices"
	"strings"

	"github.com/rsmrtk/helmsman/internal/model"
)

// The header menu groups keys by what they do instead of by their spelling, so
// the eye finds "look at it" keys in one column, "change it" keys in another and
// the app-wide ones in a third. Each group fills its own column(s).

const (
	groupInspect = iota // look at the selected row
	groupAct            // change something
	groupApp            // move around, find, theme, help
	groupOther
)

// groupOrder lists the keys of each group in the order they are shown.
var groupOrder = [][]string{
	groupInspect: {"y", "d", "l", "p", "x", "c", "o", "f", "w", "shift-j", "v", "enter"},
	groupAct:     {"a", "e", "s", "ctrl-d", "ctrl-k", "shift-f", "t", "z", "r", "u"},
	groupApp:     {"ctrl-f", "ctrl-b", "ctrl-t", "ctrl-z", "ctrl-w", "ctrl-a", "?"},
}

// minGroupSize is the smallest group that gets a column of its own; a smaller
// one is shown under the group that follows it.
const minGroupSize = 3

func groupOf(h model.MenuHint) int {
	key := strings.ToLower(h.Mnemonic)
	for g, keys := range groupOrder {
		if slices.Contains(keys, key) {
			return g
		}
	}
	if strings.HasPrefix(strings.ToLower(h.Description), "sort") || strings.HasPrefix(strings.ToLower(h.Description), "toggle") {
		return groupApp
	}

	return groupOther
}

func rank(g int, key string) int {
	if g >= len(groupOrder) {
		return 0
	}
	if i := slices.Index(groupOrder[g], strings.ToLower(key)); i >= 0 {
		return i
	}

	return len(groupOrder[g])
}

// groupHints splits the visible hints into the number keys (namespaces) and the
// columns-worth groups. hh must already be in its natural (sorted) order.
func groupHints(hh model.MenuHints) (digits model.MenuHints, groups []model.MenuHints) {
	buckets := make([]model.MenuHints, groupOther+1)
	for _, h := range hh {
		if !h.Visible {
			continue
		}
		if menuRX.MatchString(h.Mnemonic) {
			digits = append(digits, h)
			continue
		}
		g := groupOf(h)
		buckets[g] = append(buckets[g], h)
	}
	for g := range buckets {
		b := buckets[g]
		slices.SortStableFunc(b, func(x, y model.MenuHint) int { return rank(g, x.Mnemonic) - rank(g, y.Mnemonic) })
	}

	// the keys nobody grouped are shown as one tail group
	var carry model.MenuHints
	for g, b := range buckets {
		b = append(carry, b...)
		carry = nil
		if len(b) == 0 {
			continue
		}
		if len(b) < minGroupSize && g < groupOther {
			if len(groups) > 0 { // keep it next to what it relates to
				groups[len(groups)-1] = append(groups[len(groups)-1], b...)
			} else {
				carry = b
			}
			continue
		}
		groups = append(groups, b)
	}
	if len(carry) > 0 {
		groups = append(groups, carry)
	}

	return digits, groups
}

// groupedTable places every group in its own column(s) of at most maxRows rows.
func groupedTable(digits model.MenuHints, groups []model.MenuHints) (table []model.MenuHints, maxKeys []int) {
	cols := 0
	if len(digits) > 0 {
		cols = columnsFor(len(digits))
	}
	for _, g := range groups {
		cols += columnsFor(len(g))
	}
	cols = max(cols, 1)

	table = make([]model.MenuHints, maxRows)
	for r := range table {
		table[r] = make(model.MenuHints, cols)
	}
	maxKeys = make([]int, cols)

	col := 0
	place := func(hh model.MenuHints) {
		k := columnsFor(len(hh))
		rows := (len(hh) + k - 1) / k // spread evenly so the columns have the same height
		for i, h := range hh {
			c, r := col+i/rows, i%rows
			table[r][c] = h
			maxKeys[c] = max(maxKeys[c], len(h.Mnemonic))
		}
		col += k
	}
	if len(digits) > 0 {
		place(digits)
	}
	for _, g := range groups {
		place(g)
	}

	return table, maxKeys
}

// columnsFor is how many columns n hints need.
func columnsFor(n int) int {
	return max((n+maxRows-1)/maxRows, 1)
}
