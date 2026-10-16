// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package dialog

import (
	"testing"

	"github.com/derailed/tview"
	"github.com/rsmrtk/helmsman/internal/config"
	"github.com/rsmrtk/helmsman/internal/ui"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeleteDialog(t *testing.T) {
	p := ui.NewPages()

	okFunc := func(p *metav1.DeletionPropagation, f bool) {
		assert.Equal(t, propagationOptions[defaultPropagationIdx], p)
		assert.True(t, f)
	}
	ShowDelete(new(config.Dialog), p, "Yo", okFunc, func() {})

	d := p.GetPrimitive(dialogKey).(*tview.ModalForm)
	assert.NotNil(t, d)

	dismiss(p)
	assert.Nil(t, p.GetPrimitive(dialogKey))
}
