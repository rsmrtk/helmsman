// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package view_test

import (
	"testing"

	"github.com/rsmrtk/helmsman/internal/config/mock"
	"github.com/rsmrtk/helmsman/internal/view"
	"github.com/stretchr/testify/assert"
)

func TestAppNew(t *testing.T) {
	a := view.NewApp(mock.NewMockConfig(t))
	_ = a.Init("blee", 10)

	assert.Equal(t, 17, a.GetActions().Len())
}
