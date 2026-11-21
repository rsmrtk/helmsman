// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package view_test

import (
	"testing"

	"github.com/rsmrtk/helmsman/internal/client"
	"github.com/rsmrtk/helmsman/internal/dao"
	"github.com/rsmrtk/helmsman/internal/ui"
	"github.com/rsmrtk/helmsman/internal/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The nodes screen is a step of the cluster -> nodes -> namespaces -> pods drill-down. Enter continues to namespaces,
// so the pods of the highlighted node must stay reachable on their own key (`p`).
func TestNodeKeepsPodsOfNodeOnP(t *testing.T) {
	dao.MetaAccess.RegisterMeta(client.NodeGVR.String(), &metav1.APIResource{
		Name:         "nodes",
		SingularName: "node",
		Namespaced:   false,
		Kind:         "Nodes",
		Verbs:        []string{"get", "list", "watch"},
		Categories:   []string{"k9s"},
	})
	n := view.NewNode(client.NodeGVR)
	require.NoError(t, n.Init(makeCtx(t)))

	assert.Equal(t, "Nodes", n.Name())
	_, ok := n.Actions().Get(ui.KeyP)
	assert.True(t, ok, "`p` (pods of the selected node) must be bound on the nodes view")
}
