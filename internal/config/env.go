// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package config

import (
	"os"
	"strings"
)

// envNames are the settings that can be given as HELMSMAN_<NAME>. The code
// reads the original K9S_<NAME> variables, so a HELMSMAN_ value is copied over
// unless the K9S_ one is set explicitly (which keeps old setups working).
var envNames = []string{
	"CONFIG_DIR", "LOGS_DIR", "SKIN", "EDITOR", "DEFAULT_PF_ADDRESS",
	"FEATURE_GATE_NODE_SHELL", "CLIPBOARD", "OSC52_MAX",
}

// AliasEnv maps HELMSMAN_* variables onto the K9S_* ones the code reads.
func AliasEnv() {
	for _, n := range envNames {
		v := os.Getenv("HELMSMAN_" + n)
		if v == "" || strings.TrimSpace(os.Getenv("K9S_"+n)) != "" {
			continue
		}
		_ = os.Setenv("K9S_"+n, v)
	}
}
