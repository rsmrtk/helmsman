// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEveryProviderHasItsOwnTheme(t *testing.T) {
	require.Len(t, animThemes, 4, "local, aws, gcp, azure")
	icons := map[string]bool{}
	for _, prov := range []string{provLocal, "aws", "gcp", "azure"} {
		th := themeOf(prov)
		assert.Equal(t, prov, th.Key)
		assert.NotEmpty(t, th.Title, prov)
		assert.NotEmpty(t, th.Plain, prov)
		assert.NotEmpty(t, th.Logo, prov)
		require.NotNil(t, th.Shade, prov)
		assert.False(t, icons[th.Icon], "providers must not share an icon")
		icons[th.Icon] = true
	}
}

// The logos were typed by hand once and came out crooked. They are generated now; this keeps them rectangular.
func TestLogosAreRectangularAndMadeOfBlocks(t *testing.T) {
	for prov, th := range animThemes {
		w := utf8.RuneCountInString(th.Logo[0])
		assert.Len(t, th.Logo, 6, prov)
		for i, row := range th.Logo {
			assert.Equal(t, w, utf8.RuneCountInString(row), "%s row %d has a different width", prov, i)
		}
		assert.Equal(t, w, logoWidth(th.Logo), prov)
		assert.Equal(t, w, th.width(), prov)
		assert.Equal(t, len(th.Logo), th.height(), prov)
		assert.Contains(t, strings.Join(th.Logo, ""), "█", prov)
		assert.NotContains(t, strings.Join(th.Logo, ""), "\t", prov)
	}
}

func TestShadeIsAValidColorEverywhereAndActuallyMoves(t *testing.T) {
	for prov, th := range animThemes {
		w := th.width()
		moved := false
		for tt := range 200 {
			for _, solid := range []bool{true, false} {
				for r := range len(th.Logo) {
					for c := 0; c < w; c += 3 {
						col := th.Shade(c, r, w, tt, solid)
						for _, v := range []int{col.r, col.g, col.b} {
							require.GreaterOrEqual(t, v, 0, prov)
							require.LessOrEqual(t, v, 255, prov)
						}
					}
				}
			}
			if th.Shade(w/2, 2, w, tt, true) != th.Shade(w/2, 2, w, 0, true) {
				moved = true
			}
		}
		assert.True(t, moved, "%s must actually animate", prov)
	}
}

func TestEachAnimationLooksDifferent(t *testing.T) {
	w := 30
	a := themeOf("aws").Shade(10, 2, w, 7, true)
	g := themeOf("gcp").Shade(10, 2, w, 7, true)
	z := themeOf("azure").Shade(10, 2, w, 7, true)
	assert.NotEqual(t, a, g)
	assert.NotEqual(t, g, z)
	assert.NotEqual(t, a, z)
}

func TestRenderKeepsTheLogoShapeAndOnlyColorsIt(t *testing.T) {
	ui := &initUI{w: &bytes.Buffer{}, color: true}
	plain := &initUI{w: &bytes.Buffer{}, color: false}
	for prov, th := range animThemes {
		for _, final := range []bool{false, true} {
			colored := ui.render(th, 5, final)
			bare := plain.render(th, 5, final)
			require.Len(t, colored, len(th.Logo), prov)
			for i := range th.Logo {
				assert.Equal(t, indent+th.Logo[i], bare[i], "%s row %d: the shape must not change between frames", prov, i)
				assert.Contains(t, colored[i], "\x1b[38;2;", prov)
				assert.NotContains(t, bare[i], "\x1b", prov)
			}
		}
	}
}

func TestNothingAnimatesOffATerminal(t *testing.T) {
	var b bytes.Buffer
	ui := newInitUI(&b, false)
	assert.False(t, ui.anim, "a buffer is not a terminal")
	ran := false
	ui.stage("aws", func() { ran = true; ui.ok("account", "x") })
	assert.True(t, ran)
	out := b.String()
	assert.Contains(t, out, "AWS", "plain heading")
	assert.Contains(t, out, "✓ account")
	assert.NotContains(t, out, "█", "no logo when not animating")
	assert.NotContains(t, out, "\x1b")
}

// A fake terminal: animation on, color off, 120 columns (a buffer reports 80, so fits() would refuse; force the width).
type wide struct{ bytes.Buffer }

func animatingUI(w *wide) *initUI { return &initUI{w: w, anim: true} }

func TestWorkOutputIsHeldUnderTheFinishedLogoNotInsideTheAnimation(t *testing.T) {
	var w wide
	ui := animatingUI(&w)
	// a buffer reports width 80 and the widest logo needs 45: all three fit
	for _, prov := range []string{"aws", "gcp", "azure"} {
		w.Reset()
		ui.stage(prov, func() {
			ui.setStatus("listing clusters…")
			ui.ok("account", "someone @ 1")
			ui.fail("eks", "boom")
		})
		out := w.String()
		iAcc := strings.Index(out, "✓ account")
		require.GreaterOrEqual(t, iAcc, 0, prov)
		// everything the work printed comes after the last logo frame was drawn
		assert.Greater(t, iAcc, strings.LastIndex(out[:iAcc], "██"), prov)
		assert.Equal(t, 1, strings.Count(out, "✓ account"), "%s: a result line must appear exactly once", prov)
		assert.Equal(t, 1, strings.Count(out, "✗ eks"), prov)
	}
}

func TestFramesRedrawInPlaceInsteadOfPilingUp(t *testing.T) {
	var w wide
	ui := animatingUI(&w)
	th := themeOf("aws")
	ui.stage("aws", func() { ui.setStatus("x"); ui.ok("a", "b") })
	out := w.String()
	up := "\x1b[" + "7" + "A" // logo rows + status line
	assert.Contains(t, out, up, "every frame jumps back to the top of the logo block")
	frames := strings.Count(out, up)
	assert.GreaterOrEqual(t, frames, 2, "the logo is redrawn, not printed again below")
	// the heading is written once, however many frames there are
	assert.Equal(t, 1, strings.Count(out, th.Subtitle), "heading must not repeat per frame")
	assert.Contains(t, out, wrapOff, "long lines must never wrap, or redrawing in place breaks")
}

func TestStatusLineNeverExceedsTheTerminalWidth(t *testing.T) {
	ui := animatingUI(&wide{})
	th := themeOf("aws")
	ui.setStatus(strings.Repeat("a very long status message ", 20))
	line := ui.statusLine(th, nowFn())
	assert.LessOrEqual(t, utf8.RuneCountInString(stripANSI(line)), ui.termWidth(), "a status longer than the screen wraps and duplicates frames")
}

func TestStepRunsTheFunctionAndKeepsItsError(t *testing.T) {
	ui := &initUI{w: &bytes.Buffer{}}
	v, err := step(ui, "doing…", func() (int, error) { return 7, nil })
	require.NoError(t, err)
	assert.Equal(t, 7, v)
	assert.Equal(t, "doing…", ui.status)

	boom := errors.New("boom")
	_, err = step(ui, "again…", func() (string, error) { return "", boom })
	assert.ErrorIs(t, err, boom)
}

func TestCursorAndWrapAreAlwaysRestored(t *testing.T) {
	var b bytes.Buffer
	ui := &initUI{w: &b, anim: true}
	ui.begin()
	ui.end()
	assert.Equal(t, cursorHide+wrapOn+cursorShow, b.String(), "a hidden cursor and disabled wrapping must always come back")

	b.Reset()
	(&initUI{w: &b}).begin()
	assert.Empty(t, b.String(), "nothing is touched when not animating")
}

func TestIconsCanBeSwitchedToText(t *testing.T) {
	t.Setenv("HELMSMAN_ICONS", "plain")
	assert.True(t, iconsPlain())
	assert.Equal(t, "aws", themeOf("aws").icon())
	t.Setenv("HELMSMAN_ICONS", "")
	assert.Equal(t, themeOf("aws").Icon, themeOf("aws").icon())
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// A tiny terminal model: it follows only what decides whether a frame lands where the last one did.
func cursorRowsAtJumps(out string, n int) (rows []int, endRow int) {
	row := 0
	for i := range len(out) {
		switch {
		case out[i] == '\n':
			row++
		case out[i] == 0x1b && i+1 < len(out) && out[i+1] == '[':
			j := i + 2
			for j < len(out) && (out[j] >= '0' && out[j] <= '9' || out[j] == ';' || out[j] == '?') {
				j++
			}
			if j < len(out) && out[j] == 'A' {
				k := 0
				for _, d := range out[i+2 : j] {
					k = k*10 + int(d-'0')
				}
				if k == n {
					rows = append(rows, row)
				}
				row -= k
			}
			i = j //nolint:ineffassign // skips the escape sequence just consumed
		}
	}

	return rows, row
}

func TestEveryFrameStartsFromTheSameRowSoTheBlockNeverCreepsUp(t *testing.T) {
	for _, prov := range []string{"aws", "gcp", "azure"} {
		var w wide
		ui := animatingUI(&w)
		n := len(themeOf(prov).Logo) + 1
		ui.stage(prov, func() { ui.setStatus("x"); ui.ok("a", "b") })
		rows, _ := cursorRowsAtJumps(w.String(), n)
		require.GreaterOrEqual(t, len(rows), 3, prov)
		for i, r := range rows {
			assert.Equal(t, rows[0], r, "%s: frame %d starts on row %d, the first on %d: the block is drifting", prov, i, r, rows[0])
		}
	}
}

func TestFinishedLogoLeavesTheCursorRightUnderIt(t *testing.T) {
	var w wide
	ui := animatingUI(&w)
	n := len(themeOf("aws").Logo) + 1
	ui.stage("aws", func() {}) // the work prints nothing, so only the logo block is measured
	rows, end := cursorRowsAtJumps(w.String(), n)
	require.NotEmpty(t, rows)
	// Before each frame the cursor is one row below the block (its status row is the block's last row). After the final
	// frame it must sit on that status row: the results start right under the logo, with no gap and no overlap.
	assert.Equal(t, rows[0]-1, end)
}

func TestLongResultsWrapInsteadOfBeingCutOff(t *testing.T) {
	var w wide
	ui := animatingUI(&w)
	ui.stage("gcp", func() { ui.fail("GKE", "x") })
	out := w.String()
	last := strings.LastIndex(out, wrapOn)
	require.GreaterOrEqual(t, last, 0, "wrapping is switched back on")
	assert.Greater(t, strings.Index(out, "✗ GKE"), last, "the results are written with wrapping ON, so a long hint is wrapped, not truncated")
	assert.Less(t, strings.LastIndex(out, wrapOff), last, "no frame switches wrapping off after it was restored")
}

// Google's identity is four flat colors. A gradient made them muddy, so every lit cell must be exactly one of them
// (the shadow strokes are the same colors at half brightness).
func TestGCPUsesOnlyFlatGoogleColorsNeverABlend(t *testing.T) {
	pal := map[rgb]bool{gBlue: true, gRed: true, gYellow: true, gGreen: true}
	half := map[rgb]bool{}
	for c := range pal {
		half[mix(rgb{0, 0, 0}, c, 0.5)] = true
	}
	th := themeOf("gcp")
	w := th.width()
	for tt := range 200 {
		for row := range len(th.Logo) {
			for col := range w {
				assert.True(t, pal[th.Shade(col, row, w, tt, true)], "tick %d (%d,%d): a blend, not a Google color", tt, col, row)
				assert.True(t, half[th.Shade(col, row, w, tt, false)], "shadow strokes are a darker flat color")
			}
		}
	}
}

func TestGCPShowsAllFourColorsAtOnceAndTheyScroll(t *testing.T) {
	th := themeOf("gcp")
	w := th.width()
	frame := func(tt int) []rgb {
		var out []rgb
		for col := range w {
			out = append(out, th.Shade(col, 2, w, tt, true))
		}
		return out
	}
	seen := map[rgb]bool{}
	for _, c := range frame(0) {
		seen[c] = true
	}
	assert.Len(t, seen, 4, "all four Google colors are visible in a single frame")

	// scrolling right: what was at column c one tick ago is now at column c+1
	a, b := frame(10), frame(11)
	for col := range w - 1 {
		assert.Equal(t, a[col], b[col+1], "the pattern shifts one cell to the right per tick (col %d)", col)
	}
	assert.NotEqual(t, frame(0), frame(5), "the picture changes over time")
}

func TestGCPBandEdgesLeanDiagonally(t *testing.T) {
	th := themeOf("gcp")
	w := th.width()
	// the same column has a different color a few rows down (edges are not vertical)
	differs := false
	for col := range w {
		if th.Shade(col, 0, w, 3, true) != th.Shade(col, 5, w, 3, true) {
			differs = true
		}
	}
	assert.True(t, differs, "band edges must lean, not stand vertical")
}

func TestEveryLogoFitsInEightyColumns(t *testing.T) {
	ui := &initUI{w: &bytes.Buffer{}, anim: true}
	for prov, th := range animThemes {
		assert.True(t, ui.fits(th), "%s logo fits in 80 columns", prov)
	}
}

func TestHeadingIsOneLineWhenTheTerminalCannotScale(t *testing.T) {
	var w wide
	ui := animatingUI(&w)
	ui.scale = 1
	ui.heading(themeOf(provGCP))
	out := w.String()
	assert.NotContains(t, out, "\x1b]66", "an ignored OSC 66 would swallow the glyph, so it is never sent unless support is known")
	assert.Contains(t, out, themeOf(provGCP).Icon)
	assert.Contains(t, out, "Google Cloud")
	assert.Equal(t, 2, strings.Count(out, "\n"), "one line plus a blank line")
}

func TestHeadingScalesTheGlyphWithTheTextSizingProtocol(t *testing.T) {
	for prov, th := range animThemes {
		var w wide
		ui := animatingUI(&w)
		ui.scale = 2
		ui.heading(th)
		out := w.String()
		assert.Contains(t, out, "\x1b]66;s=2;"+th.Icon+"\a", "%s: the glyph is drawn at x2", prov)
		assert.Equal(t, 1, strings.Count(out, th.Icon), "%s: the glyph appears once, not once per row", prov)
		assert.True(t, strings.HasPrefix(out, "\n\n\x1b[2A"), "%s: room is made for the glyph rows first, then the cursor comes back up", prov)
		out = strings.TrimPrefix(out, "\n\n\x1b[2A")
		lines := strings.Split(out, "\n")
		require.GreaterOrEqual(t, len(lines), 3, prov)
		assert.Contains(t, lines[0], th.Title, "%s: the name sits beside the top of the glyph", prov)
		assert.Contains(t, lines[1], th.Subtitle, "%s: the service sits beside the second row", prov)
		// writing spaces under the scaled glyph would erase its lower half: row two must move with cursor-forward instead
		assert.True(t, strings.HasPrefix(lines[1], "\x1b["), "%s: row two starts with a cursor move, not spaces", prov)
		assert.Contains(t, lines[1], "C"+th.Subtitle[:0])
		assert.Equal(t, 3, strings.Count(out, "\n"), "%s: two rows plus a blank line (the room-making newlines were trimmed above)", prov)
	}
}

func TestScaleFromTheEnvironment(t *testing.T) {
	var b bytes.Buffer
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("TERM", "xterm-256color")

	t.Setenv("HELMSMAN_ICON_SCALE", "1")
	assert.Equal(t, 1, iconScale(&b))
	t.Setenv("HELMSMAN_ICON_SCALE", "off")
	assert.Equal(t, 1, iconScale(&b))
	t.Setenv("HELMSMAN_ICON_SCALE", "3")
	assert.Equal(t, 3, iconScale(&b), "a number from 2 to 4 forces that scale")
	t.Setenv("HELMSMAN_ICON_SCALE", "9")
	assert.Equal(t, 1, iconScale(&b), "out of range is ignored; a buffer is no terminal, so no scaling")

	t.Setenv("HELMSMAN_ICON_SCALE", "")
	assert.Equal(t, 1, iconScale(&b), "an unknown terminal that is not a tty is not probed and not scaled")
	t.Setenv("TERM", "xterm-kitty")
	assert.Equal(t, defaultIconScale, iconScale(&b), "kitty is recognized from TERM")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("KITTY_WINDOW_ID", "2")
	assert.Equal(t, defaultIconScale, iconScale(&b), "and from KITTY_WINDOW_ID")
}

func TestParseCursorPositionReport(t *testing.T) {
	col, ok := parseCPR([]byte("\x1b[12;34R"))
	assert.True(t, ok)
	assert.Equal(t, 34, col)
	_, ok = parseCPR([]byte("garbage"))
	assert.False(t, ok)
	_, ok = parseCPR(nil)
	assert.False(t, ok)
	col, ok = parseCPR([]byte("xx\x1b[1;7Ryy"))
	assert.True(t, ok, "noise around the report is tolerated")
	assert.Equal(t, 7, col)
}

func TestScaledGlyphSequence(t *testing.T) {
	assert.Equal(t, "\x1b]66;s=2;X\a", scaledGlyph("X", 2))
}

func TestPlainIconsNeverScale(t *testing.T) {
	t.Setenv("HELMSMAN_ICONS", "plain")
	t.Setenv("TERM", "xterm-kitty")
	ui := newInitUI(&bytes.Buffer{}, false)
	assert.Equal(t, 1, ui.scale)
}

func TestKubernetesBlockIsNamedKubernetesNotLocal(t *testing.T) {
	assert.Equal(t, "Kubernetes", themeOf(provLocal).Title)
	assert.Equal(t, "Kubernetes", providerTitle[provLocal])
	assert.NotContains(t, themeOf(provLocal).Subtitle+themeOf(provLocal).Title, "Local")
}

func TestTheKubernetesGradientFlowsButTheShapeStaysStill(t *testing.T) {
	th := themeOf(provLocal)
	w := th.width()
	changed := false
	for tt := range 40 {
		if th.Shade(10, 4, w, tt, true) != th.Shade(10, 4, w, 0, true) {
			changed = true
		}
	}
	assert.True(t, changed, "the colors shimmer over time")
	// neighboring cells differ a little, not a lot: it is a smooth gradient, not noise and not flat bands
	for c := range w - 1 {
		a, b := th.Shade(c, 3, w, 7, true), th.Shade(c+1, 3, w, 7, true)
		assert.InDelta(t, a.r, b.r, 40, "col %d", c)
		assert.InDelta(t, a.g, b.g, 40, "col %d", c)
		assert.InDelta(t, a.b, b.b, 40, "col %d", c)
	}
	// the shadow strokes are the same gradient, darker
	solid, shadow := th.Shade(20, 4, w, 3, true), th.Shade(20, 4, w, 3, false)
	assert.Less(t, shadow.b, solid.b)
	// every color is valid
	for tt := range 120 {
		for r := range th.Logo {
			for c := range w {
				col := th.Shade(c, r, w, tt, true)
				assert.True(t, col.r >= 0 && col.r <= 255 && col.g >= 0 && col.g <= 255 && col.b >= 0 && col.b <= 255)
			}
		}
	}
}
