// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package view

import (
	"testing"

	"github.com/rsmrtk/helmsman/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestYaml(t *testing.T) {
	uu := []struct {
		s, e string
	}{
		{
			`api: fred
		   version: v1`,
			`[#7c9ee6::b]api[#ffffff::-]: [#ffefd5::]fred
		   [#7c9ee6::b]version[#ffffff::-]: [#ffefd5::]v1`,
		},
		{
			`api: <<<"search_0">>>fred<<<"">>>
		   version: v1`,
			`[#7c9ee6::b]api[#ffffff::-]: [#ffefd5::]["search_0"]fred[""]
		   [#7c9ee6::b]version[#ffffff::-]: [#ffefd5::]v1`,
		},
		{
			`api:
			version: v1`,
			`[#7c9ee6::b]api[#ffffff::-]:
			[#7c9ee6::b]version[#ffffff::-]: [#ffefd5::]v1`,
		},
		{
			"      fred:blee",
			"[#ffefd5::]      fred:blee",
		},
		{
			"fred blee: blee",
			"[#7c9ee6::b]fred blee[#ffffff::-]: [#ffefd5::]blee",
		},
		{
			"Node-Selectors:  <none>",
			"[#7c9ee6::b]Node-Selectors[#ffffff::-]: [#ffefd5::] <none>",
		},
		{
			"fred.blee:  <none>",
			"[#7c9ee6::b]fred.blee[#ffffff::-]: [#ffefd5::] <none>",
		},
		{
			"certmanager.k8s.io/cluster-issuer: nameOfClusterIssuer",
			"[#7c9ee6::b]certmanager.k8s.io/cluster-issuer[#ffffff::-]: [#ffefd5::]nameOfClusterIssuer",
		},
		{
			"Message: Pod The node was low on resource: [DiskPressure].",
			"[#7c9ee6::b]Message[#ffffff::-]: [#ffefd5::]Pod The node was low on resource: [DiskPressure[].",
		},
		{
			`data: "<<<"`,
			`[#7c9ee6::b]data[#ffffff::-]: [#ffefd5::]"<<<"`,
		},
	}

	s := config.NewStyles()
	for _, u := range uu {
		assert.Equal(t, u.e, colorizeYAML(s.Views().Yaml, u.s))
	}
}
