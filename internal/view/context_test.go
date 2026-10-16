// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package view_test

import (
	"testing"

	"github.com/rsmrtk/helmsman/internal/client"
	"github.com/rsmrtk/helmsman/internal/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContext(t *testing.T) {
	ctx := view.NewContext(client.CtGVR)

	require.NoError(t, ctx.Init(makeCtx(t)))
	assert.Equal(t, "Contexts", ctx.Name())
	assert.Len(t, ctx.Hints(), 11)
}
