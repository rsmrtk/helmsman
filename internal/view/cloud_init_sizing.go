// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"io"
	"os"
	"regexp"
	"strconv"
)

// A font glyph is one terminal cell and a program cannot make it bigger. The Kitty text sizing protocol (OSC 66) can: it
// draws a piece of text at a multiple of the cell size. kitty supports it; other terminals either support it too or
// ignore the sequence, and an ignored sequence would also swallow the glyph, so the scaled heading is used only where
// support is known or has just been proven:
//
//  1. HELMSMAN_ICON_SCALE=1 (or off) never scales; a number from 2 to 4 forces that scale;
//  2. kitty is recognized from its environment (KITTY_WINDOW_ID, TERM=xterm-kitty);
//  3. any other terminal is asked: a two-cell wide space is printed and the cursor position is read back. A terminal that
//     understands the protocol moves the cursor by two cells. This is the detection the protocol itself recommends.
//
// Anywhere else (a pipe, Windows, a terminal that does not answer) the heading is the plain one-line version.

const defaultIconScale = 2

// iconScale returns the scale to draw the heading glyph at: 1 means "do not scale".
func iconScale(w io.Writer) int {
	switch v := os.Getenv("HELMSMAN_ICON_SCALE"); v {
	case "":
	case "1", "0", "off", "no", "false":
		return 1
	default:
		if n, err := strconv.Atoi(v); err == nil && n >= 2 && n <= 4 {
			return n
		}
	}
	if os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("TERM") == "xterm-kitty" {
		return defaultIconScale
	}
	if probeTextSizing(w) {
		return defaultIconScale
	}

	return 1
}

// scaledGlyph is the OSC 66 sequence that draws text at scale s: ESC ] 66 ; s=<n> ; <text> BEL.
func scaledGlyph(glyph string, s int) string {
	return "\x1b]66;s=" + strconv.Itoa(s) + ";" + glyph + "\a"
}

var cprRe = regexp.MustCompile(`\x1b\[(\d+);(\d+)R`)

// parseCPR reads a cursor position report (ESC [ row ; col R) and returns the column.
func parseCPR(b []byte) (col int, ok bool) {
	m := cprRe.FindSubmatch(b)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(string(m[2]))

	return n, err == nil
}
