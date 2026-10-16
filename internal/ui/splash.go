// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/derailed/tcell/v2"
	"github.com/derailed/tview"
	"github.com/rsmrtk/helmsman/internal/config"
)

// LogoSmall helmsman small logo (ANSI Shadow face).
var LogoSmall = []string{
	`██╗  ██╗███╗   ███╗`,
	`██║  ██║████╗ ████║`,
	`███████║██╔████╔██║`,
	`██╔══██║██║╚██╔╝██║`,
	`██║  ██║██║ ╚═╝ ██║`,
	`╚═╝  ╚═╝╚═╝     ╚═╝`,
}

// LogoBig helmsman big logo for splash page.
var LogoBig = []string{
	`██╗  ██╗███████╗██╗     ███╗   ███╗███████╗███╗   ███╗ █████╗ ███╗   ██╗`,
	`██║  ██║██╔════╝██║     ████╗ ████║██╔════╝████╗ ████║██╔══██╗████╗  ██║`,
	`███████║█████╗  ██║     ██╔████╔██║███████╗██╔████╔██║███████║██╔██╗ ██║`,
	`██╔══██║██╔══╝  ██║     ██║╚██╔╝██║╚════██║██║╚██╔╝██║██╔══██║██║╚██╗██║`,
	`██║  ██║███████╗███████╗██║ ╚═╝ ██║███████║██║ ╚═╝ ██║██║  ██║██║ ╚████║`,
	`╚═╝  ╚═╝╚══════╝╚══════╝╚═╝     ╚═╝╚══════╝╚═╝     ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝`,
}

// Splash represents a splash screen.
type Splash struct {
	*tview.Flex

	logo   *tview.TextView
	styles *config.Styles
}

// NewSplash instantiates a new splash screen with product and company info.
func NewSplash(styles *config.Styles, version string) *Splash {
	s := Splash{Flex: tview.NewFlex(), styles: styles}
	s.SetBackgroundColor(styles.BgColor())

	s.logo = tview.NewTextView()
	s.logo.SetDynamicColors(true)
	s.logo.SetTextAlign(tview.AlignCenter)
	// The art must never wrap: a wrapped row looks like a broken logo.
	s.logo.SetWrap(false)
	s.logo.SetWordWrap(false)
	s.logo.SetBackgroundColor(styles.BgColor())
	s.frame(0)

	vers := tview.NewTextView()
	vers.SetDynamicColors(true)
	vers.SetTextAlign(tview.AlignCenter)
	vers.SetBackgroundColor(styles.BgColor())
	s.layoutRev(vers, version, styles)

	s.SetDirection(tview.FlexRow)
	s.AddItem(nil, 0, 1, false)
	s.AddItem(s.logo, len(LogoBig), 0, false)
	s.AddItem(vers, 2, 0, false)
	s.AddItem(nil, 0, 1, false)

	return &s
}

// Animate sweeps a light band across the logo, revealing it left to right.
// Every frame is drawn through queue, so it is safe to call from a goroutine.
// The last frame is the fully lit logo, so the screen never switches mid-sweep.
func (s *Splash) Animate(queue func(func()), d time.Duration) {
	const fps = 25 * time.Millisecond
	go func() {
		start := time.Now()
		for {
			p := float64(time.Since(start)) / float64(d)
			if p > 1 {
				p = 1
			}
			// ease-out: fast start, the band slows down as it reaches the end
			e := 1 - (1-p)*(1-p)*(1-p)
			queue(func() { s.frame(e) })
			if p >= 1 {
				return
			}
			time.Sleep(fps)
		}
	}()
}

// frame draws the logo at progress p in [0, 1]; the glow trails the reveal edge.
func (s *Splash) frame(p float64) {
	const band = 10.0
	width := 0
	for _, l := range LogoBig {
		width = max(width, len([]rune(l)))
	}
	edge := p * (float64(width) + band)
	base := s.styles.Body().LogoColor.Color()
	bg := s.styles.BgColor()
	glow := mix(base, tcell.ColorWhite, 0.75)

	var sb strings.Builder
	for i, line := range LogoBig {
		for x, r := range []rune(line) {
			d := edge - float64(x)
			if d < 0 || r == ' ' {
				sb.WriteRune(' ')
				continue
			}
			fg := base
			if d < band {
				fg = mix(glow, base, d/band)
			}
			if strings.ContainsRune(shadowRunes, r) {
				fg = mix(bg, fg, 0.45)
			}
			fmt.Fprintf(&sb, "[%s::b]%s", hexOf(fg), tview.Escape(string(r)))
		}
		if i+1 < len(LogoBig) {
			sb.WriteString("\n")
		}
	}
	s.logo.SetText(sb.String())
}

func (*Splash) layoutRev(t *tview.TextView, rev string, styles *config.Styles) {
	dim := mix(styles.BgColor(), styles.Body().FgColor.Color(), 0.7)
	_, _ = fmt.Fprintf(t, "\n[%s::]kubernetes cluster navigator  [%s::b]%s", hexOf(dim), hexOf(styles.Body().LogoColor.Color()), rev)
}
