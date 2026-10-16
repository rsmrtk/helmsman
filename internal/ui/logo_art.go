// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package ui

import (
	"fmt"
	"strings"

	"github.com/derailed/tcell/v2"
	"github.com/derailed/tview"
)

// shadowRunes are the box-drawing strokes of the ANSI Shadow face; they are
// drawn a shade darker than the solid blocks so the letters look lit from the top left.
const shadowRunes = "╗╔╝╚═║"

// hexOf renders a colour as a tview colour tag value.
func hexOf(c tcell.Color) string {
	return fmt.Sprintf("#%06x", c.TrueColor().Hex())
}

// mix blends a toward b by t (0 keeps a, 1 gives b).
func mix(a, b tcell.Color, t float64) tcell.Color {
	ar, ag, ab := a.TrueColor().RGB()
	br, bg, bb := b.TrueColor().RGB()
	lerp := func(x, y int32) int32 { return x + int32(float64(y-x)*t) }
	return tcell.NewRGBColor(lerp(ar, br), lerp(ag, bg), lerp(ab, bb))
}

// artLine colours one line of block-letter art: solid blocks in solid, the
// shadow strokes in shadow. Spaces take the colour of the run they sit in, and
// each run is escaped as a whole so literal brackets in custom art survive.
func artLine(line, solid, shadow string) string {
	var sb strings.Builder
	var run strings.Builder
	cur := solid
	flush := func() {
		if run.Len() == 0 {
			return
		}
		fmt.Fprintf(&sb, "[%s::b]%s", cur, tview.Escape(run.String()))
		run.Reset()
	}
	for _, r := range line {
		col := cur
		if strings.ContainsRune(shadowRunes, r) {
			col = shadow
		} else if r != ' ' {
			col = solid
		}
		if col != cur {
			flush()
			cur = col
		}
		run.WriteRune(r)
	}
	flush()
	return sb.String()
}

// twoToneArt renders art with the columns before split in the first tone and
// the rest in the second; a negative split uses a single tone.
func twoToneArt(lines []string, split int, first, second, bg tcell.Color) string {
	var sb strings.Builder
	for i, line := range lines {
		runes := []rune(line)
		cut := split
		if cut < 0 || cut > len(runes) {
			cut = len(runes)
		}
		sb.WriteString(artLine(string(runes[:cut]), hexOf(first), hexOf(mix(bg, first, 0.45))))
		if cut < len(runes) {
			sb.WriteString(artLine(string(runes[cut:]), hexOf(second), hexOf(mix(bg, second, 0.45))))
		}
		if i+1 < len(lines) {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
