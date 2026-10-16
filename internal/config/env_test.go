// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package config_test

import (
	"os"
	"testing"

	"github.com/rsmrtk/helmsman/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestAliasEnv(t *testing.T) {
	t.Setenv("HELMSMAN_SKIN", "helmsman-ember")
	t.Setenv("K9S_SKIN", "")
	t.Setenv("HELMSMAN_EDITOR", "nvim")
	t.Setenv("K9S_EDITOR", "vim") // an explicit old setting wins
	config.AliasEnv()

	assert.Equal(t, "helmsman-ember", os.Getenv("K9S_SKIN"))
	assert.Equal(t, "vim", os.Getenv("K9S_EDITOR"))
}
