// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

//go:build windows

package view

import "io"

// probeTextSizing: no probing on Windows; the heading stays on one line unless HELMSMAN_ICON_SCALE forces a scale.
func probeTextSizing(io.Writer) bool { return false }
