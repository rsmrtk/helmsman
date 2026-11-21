// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

//go:build !windows

package view

import (
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const probeTimeout = 300 * time.Millisecond

// probeTextSizing asks the terminal whether it implements the text sizing protocol: print a space two cells wide and
// check that the cursor moved two columns. Needs a real terminal on both ends; anything else answers "no".
func probeTextSizing(w io.Writer) bool {
	out, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(out.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return false
	}
	in := int(os.Stdin.Fd())
	old, err := term.MakeRaw(in)
	if err != nil {
		return false
	}
	defer func() { _ = term.Restore(in, old) }()

	before, ok := cursorColumn(out)
	if !ok {
		return false
	}
	fmt.Fprint(out, "\x1b]66;w=2; \a")
	after, ok := cursorColumn(out)
	fmt.Fprint(out, "\r"+eraseLine)

	return ok && after-before == 2
}

// cursorColumn asks for the cursor position and reads the answer, giving up after probeTimeout.
func cursorColumn(out *os.File) (int, bool) {
	fmt.Fprint(out, "\x1b[6n")
	deadline := time.Now().Add(probeTimeout)
	var buf []byte
	one := make([]byte, 1)
	for time.Now().Before(deadline) {
		fds := []unix.PollFd{{Fd: int32(os.Stdin.Fd()), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, int(time.Until(deadline).Milliseconds()))
		if err != nil || n == 0 {
			break
		}
		if k, err := os.Stdin.Read(one); err != nil || k == 0 {
			break
		}
		buf = append(buf, one[0])
		if one[0] == 'R' {
			break
		}
	}

	return parseCPR(buf)
}
