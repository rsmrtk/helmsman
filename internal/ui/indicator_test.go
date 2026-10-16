// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package ui_test

import (
	"testing"

	"github.com/rsmrtk/helmsman/internal/config"
	"github.com/rsmrtk/helmsman/internal/config/mock"
	"github.com/rsmrtk/helmsman/internal/ui"
	"github.com/stretchr/testify/assert"
)

func TestIndicatorReset(t *testing.T) {
	i := ui.NewStatusIndicator(ui.NewApp(mock.NewMockConfig(t), ""), config.NewStyles())
	i.SetPermanent("Blee")
	i.Info("duh")
	i.Reset()

	assert.Equal(t, "Blee\n", i.GetText(false))
}

func TestIndicatorInfo(t *testing.T) {
	i := ui.NewStatusIndicator(ui.NewApp(mock.NewMockConfig(t), ""), config.NewStyles())
	i.Info("Blee")

	assert.Equal(t, "[lawngreen::b] <Blee> \n", i.GetText(false))
}

func TestIndicatorWarn(t *testing.T) {
	i := ui.NewStatusIndicator(ui.NewApp(mock.NewMockConfig(t), ""), config.NewStyles())
	i.Warn("Blee")

	assert.Equal(t, "[mediumvioletred::b] <Blee> \n", i.GetText(false))
}

func TestIndicatorErr(t *testing.T) {
	i := ui.NewStatusIndicator(ui.NewApp(mock.NewMockConfig(t), ""), config.NewStyles())
	i.Err("Blee")

	assert.Equal(t, "[orangered::b] <Blee> \n", i.GetText(false))
}
