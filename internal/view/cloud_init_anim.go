// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

// While `helmsman init` waits for a cloud CLI, the provider's own logo is the loading animation. It is drawn once, in
// place, and every frame only recolors it, so nothing is repeated in the scrollback:
//
//	AWS          a beam of light sweeps across the letters, the block faces glow, the shadows stay dark
//	Google Cloud four flat Google colors as hard-edged diagonal bands scrolling through the logo, no gradients
//	Azure        a wave of blue rolls through the letters, deep blue to cyan
//
// Everything the work prints while the logo animates is held back and written under the finished logo, so results never
// tear the animation. Animation runs only on a terminal wide enough for the logo (and not with HELMSMAN_NO_ANIM=1 or
// --no-animation); everywhere else the output is the same plain text as before. Icons are Nerd Font glyphs;
// HELMSMAN_ICONS=plain swaps them for text.

const (
	cursorHide = "\x1b[?25l"
	cursorShow = "\x1b[?25h"
	wrapOff    = "\x1b[?7l" // never let a long line wrap: a wrapped line breaks redrawing in place
	wrapOn     = "\x1b[?7h"
	eraseLine  = "\x1b[2K"
	tick       = 70 * time.Millisecond
	minShow    = 1400 * time.Millisecond // a provider that answers instantly still gets its animation seen
	indent     = "  "
)

// Provider keys, used by the init command, the themes and the tests.
const (
	provAWS   = "aws"
	provGCP   = "gcp"
	provAzure = "azure"
)

type rgb struct{ r, g, b int }

func (c rgb) fg() string { return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c.r, c.g, c.b) }

func mix(a, b rgb, t float64) rgb {
	t = math.Max(0, math.Min(1, t))

	return rgb{int(float64(a.r) + (float64(b.r)-float64(a.r))*t), int(float64(a.g) + (float64(b.g)-float64(a.g))*t), int(float64(a.b) + (float64(b.b)-float64(a.b))*t)}
}

func paintRGB(c rgb, s string, color bool) string {
	if !color {
		return s
	}

	return c.fg() + s + cReset
}

var (
	awsOrange  = rgb{255, 153, 0}
	awsBright  = rgb{255, 232, 180}
	awsDark    = rgb{86, 52, 8}
	gBlue      = rgb{66, 133, 244}
	gRed       = rgb{234, 67, 53}
	gYellow    = rgb{251, 188, 5}
	gGreen     = rgb{52, 168, 83}
	azureDeep  = rgb{0, 70, 140}
	azureBlue  = rgb{0, 120, 212}
	azureLight = rgb{110, 235, 255}
)

// animTheme is everything that makes one provider look like itself. Shade colors one cell of the logo: col and row are
// the cell, w the logo width, t the frame number, solid whether the cell is a block face (█) or a shadow stroke.
type animTheme struct {
	Key, Title, Subtitle, Plain, Icon string
	Accent                            rgb
	Logo                              []string // the lettering, drawn in block letters
	Shade                             func(col, row, w, t int, solid bool) rgb
}

var animThemes = map[string]*animTheme{
	provAWS: {
		Key: provAWS, Title: "AWS", Subtitle: "Amazon Web Services · EKS", Icon: "\U000F0E0F", Plain: provAWS, Accent: awsOrange,
		Logo: logoAWS, Shade: shadeAWS,
	},
	provGCP: {
		Key: provGCP, Title: "Google Cloud", Subtitle: "Google Cloud · GKE", Icon: "\U000F11F6", Plain: provGCP, Accent: gBlue,
		Logo: logoGCP, Shade: shadeGCP,
	},
	provAzure: {
		Key: provAzure, Title: "Azure", Subtitle: "Microsoft Azure · AKS", Icon: "\U000F0805", Plain: provAzure, Accent: azureBlue,
		Logo: logoAzure, Shade: shadeAzure,
	},
}

func themeOf(prov string) *animTheme { return animThemes[prov] }

// shadeAWS: a beam sweeps left to right and back. Block faces light up near it; shadow strokes only warm a little.
func shadeAWS(col, _, w, t int, solid bool) rgb {
	span := w + 12
	p := t % (2 * span)
	if p >= span {
		p = 2*span - p
	}
	d := float64(col) - (float64(p) - 6)
	glow := math.Exp(-(d * d) / 18)
	base := mix(awsDark, awsOrange, 0.42)
	if !solid {
		return mix(awsDark, awsOrange, 0.12+0.35*glow)
	}

	return mix(mix(base, awsOrange, 0.35), awsBright, glow)
}

// shadeGCP: four flat Google colors as hard-edged diagonal bands that scroll through the logo, like Google's own
// indeterminate progress bar. No blending: every lit cell is exactly one of the four colors; the shadow strokes are the
// same color at half brightness. All four colors are on screen at once and the whole pattern keeps moving.
func shadeGCP(col, row, _, t int, solid bool) rgb {
	pal := []rgb{gBlue, gRed, gYellow, gGreen}
	const bandWidth = 5
	// the diagonal: lower rows are shifted, so the band edges lean; the minus sign makes the bands travel to the right
	pos := col + row*2 - t
	band := ((pos/bandWidth)%len(pal) + len(pal)) % len(pal)
	c := pal[band]
	if !solid {
		return mix(rgb{0, 0, 0}, c, 0.5)
	}

	return c
}

// shadeAzure: a wave of light rolls through the letters.
func shadeAzure(col, row, _, t int, solid bool) rgb {
	h := 0.5 + 0.5*math.Sin(float64(col)*0.30-float64(t)*0.34+float64(row)*0.45)
	if !solid {
		return mix(azureDeep, azureBlue, h*0.7)
	}

	return mix(azureBlue, azureLight, h)
}

// ---------------------------------------------------------------------------------------------------------------------

func animationAllowed(w any, noAnim bool) bool {
	f, ok := w.(*os.File)
	if !ok || noAnim || os.Getenv("HELMSMAN_NO_ANIM") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}

	return isTerminal(f)
}

func iconsPlain() bool { return strings.EqualFold(os.Getenv("HELMSMAN_ICONS"), "plain") }

func (th *animTheme) icon() string {
	if iconsPlain() {
		return th.Plain
	}

	return th.Icon
}

func (th *animTheme) width() int { return logoWidth(th.Logo) }

func logoWidth(logo []string) int {
	w := 0
	for _, l := range logo {
		if n := utf8.RuneCountInString(l); n > w {
			w = n
		}
	}

	return w
}

// termWidth is the terminal's column count, or 80 when it cannot be read.
func (u *initUI) termWidth() int {
	if f, ok := u.w.(*os.File); ok {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
			return w
		}
	}

	return 80
}

// fits: the logo must fit with its indent and a margin, else the animation is skipped (a wrapped row cannot be redrawn).
func (u *initUI) fits(logo []string) bool { return u.termWidth() >= logoWidth(logo)+len(indent)+2 }

// render returns the logo as colored lines for frame t. final draws it fully lit in the provider's accent color.
func (u *initUI) render(th *animTheme, logo []string, t int, final bool) []string {
	lines := make([]string, len(logo))
	for r, row := range logo {
		var b strings.Builder
		b.WriteString(indent)
		runes := []rune(row)
		for c := range runes {
			ch := runes[c]
			if ch == ' ' {
				b.WriteRune(ch)

				continue
			}
			col := th.Accent
			if !final {
				col = th.Shade(c, r, len(runes), t, ch == '█')
			} else if ch != '█' {
				col = mix(rgb{40, 40, 40}, th.Accent, 0.55)
			}
			if u.color {
				b.WriteString(col.fg())
			}
			b.WriteRune(ch)
		}
		if u.color {
			b.WriteString(cReset)
		}
		lines[r] = b.String()
	}

	return lines
}

// setStatus is the line under the logo: what the work is doing right now.
func (u *initUI) setStatus(msg string) {
	u.mu.Lock()
	u.status = msg
	u.mu.Unlock()
}

func (u *initUI) statusLine(th *animTheme, start time.Time) string {
	u.mu.Lock()
	msg := u.status
	u.mu.Unlock()
	tail := ""
	if d := time.Since(start); d >= 1500*time.Millisecond {
		tail = fmt.Sprintf("  %.1fs", d.Seconds())
	}
	// room for the indent, the icon, a space and a margin of two; an over-long line would wrap and break redrawing
	room := u.termWidth() - len(indent) - 1 - 1 - 2 - utf8.RuneCountInString(tail)
	if r := []rune(msg); len(r) > room && room > 1 {
		msg = string(r[:room-1]) + "…"
	}

	return indent + paintRGB(th.Accent, th.icon(), u.color) + " " + u.paint(cDim, msg+tail)
}

// heading is the provider's glyph with its name and service. Normally one line. Where the terminal can scale text (kitty)
// the glyph is drawn at u.scale times its size, spanning that many rows, with the name beside its top and the service
// beside the next row:
//
//	[glyph x2]  Google Cloud
//	[glyph x2]  Google Cloud · GKE
//
// Spaces must not be written under the scaled glyph (they would erase it), so later rows move right with "cursor forward".
func (u *initUI) heading(th *animTheme) {
	if u.scale < 2 {
		u.printf("%s%s  %s  %s\n\n", indent, paintRGB(th.Accent, th.icon(), u.color), u.paint(cBold, th.Title), u.paint(cDim, th.Subtitle))

		return
	}
	const gap = 2
	// make sure the rows the glyph will span exist even at the bottom of the window (the line feeds scroll if they must),
	// then come back up: the net movement is zero.
	u.printf("%s\x1b[%dA", strings.Repeat("\n", u.scale), u.scale)
	over := fmt.Sprintf("\x1b[%dC", len(indent)+u.scale+gap) // clear of the glyph, which is u.scale columns wide
	glyph := scaledGlyph(th.icon(), u.scale)
	if u.color {
		glyph = th.Accent.fg() + glyph + cReset
	}
	for r := range u.scale {
		switch r {
		case 0:
			u.printf("%s%s%s%s\n", indent, glyph, strings.Repeat(" ", gap), u.paint(cBold, th.Title))
		case 1:
			u.printf("%s%s\n", over, u.paint(cDim, th.Subtitle))
		default:
			u.printf("\n")
		}
	}
	u.printf("\n")
}

// stage runs work for one provider. When animating, the provider's heading and logo are drawn and animated in place
// while work runs; anything work prints is held and written under the logo afterwards.
func (u *initUI) stage(prov string, work func()) {
	th, ok := animThemes[prov]
	if !ok || !u.anim {
		u.provider(providerTitle[prov])
		work()

		return
	}
	logo := th.Logo
	if !u.fits(logo) {
		u.provider(providerTitle[prov])
		work()

		return
	}

	u.heading(th)
	n := len(logo) + 1 // logo rows + the status line
	u.mu.Lock()
	u.capture = true
	u.buf.Reset()
	u.mu.Unlock()

	start := time.Now()
	draw := func(t int, final bool) {
		var b bytes.Buffer
		b.WriteString(wrapOff)
		for _, l := range u.render(th, logo, t, final) {
			b.WriteString(eraseLine + "\r" + l + "\n")
		}
		// Every frame must end with the cursor on the SAME row (one below the block), or the next "jump up" lands one
		// row higher each time and the block creeps up the screen, repainting what is above it.
		if final {
			b.WriteString(eraseLine + "\r") // the status row is blanked and becomes the first row of what follows
		} else {
			b.WriteString(eraseLine + "\r" + u.statusLine(th, start) + "\n")
		}
		u.writeRaw(b.String())
	}

	u.writeRaw(strings.Repeat("\n", n)) // reserve the block, then jump back to its top for every frame
	up := func() { u.writeRaw(fmt.Sprintf("\x1b[%dA\r", n)) }

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(tick)
		defer t.Stop()
		for i := 0; ; i++ {
			up()
			draw(i, false)
			select {
			case <-done:
				return
			case <-t.C:
			}
		}
	}()

	work()
	if rest := minShow - time.Since(start); rest > 0 {
		time.Sleep(rest)
	}
	close(done)
	wg.Wait()
	up()
	draw(0, true)
	u.writeRaw(wrapOn) // results below may be longer than the screen and must wrap, not be cut off

	u.mu.Lock()
	u.capture = false
	held := u.buf.String()
	u.buf.Reset()
	u.mu.Unlock()
	u.writeRaw(held)
}

// step tells the status line what is happening, then runs f.
func step[T any](u *initUI, msg string, f func() (T, error)) (T, error) {
	u.setStatus(msg)

	return f()
}

func (u *initUI) writeRaw(s string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	fmt.Fprint(u.w, s)
}

// begin hides the cursor for the duration of the animation; end brings it back and re-enables line wrapping (a stage
// switches wrapping off only while a logo animates). Always defer end: a hidden cursor or disabled wrapping would stay
// in the user's shell after an interrupt.
func (u *initUI) begin() {
	if u.anim {
		fmt.Fprint(u.w, cursorHide)
	}
}

func (u *initUI) end() {
	if u.anim {
		fmt.Fprint(u.w, wrapOn+cursorShow)
	}
}

var nowFn = time.Now
