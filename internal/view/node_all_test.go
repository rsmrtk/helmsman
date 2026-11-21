// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package view

import (
	"testing"

	"github.com/rsmrtk/helmsman/internal/client"
	"github.com/rsmrtk/helmsman/internal/model1"
	"github.com/stretchr/testify/assert"
)

func nodesTable() *model1.TableData {
	h := model1.Header{{Name: "NAME"}, {Name: "STATUS"}, {Name: "PODS"}, {Name: "AGE"}}
	td := model1.NewTableDataFull(client.NodeGVR, "", h, model1.NewRowEvents(4))
	td.AddRow(model1.RowEvent{Row: model1.Row{ID: "n1", Fields: model1.Fields{"n1", "Ready", "13", "1h"}}})
	td.AddRow(model1.RowEvent{Row: model1.Row{ID: "n2", Fields: model1.Fields{"n2", "Ready", "20", "1h"}}})

	return td
}

func TestAddAllNodesRow(t *testing.T) {
	td := nodesTable()
	addAllNodesRow(td)

	re, ok := td.FindRow(nodeAll)
	assert.True(t, ok, "the synthetic row must exist")
	assert.Equal(t, 3, td.RowCount())
	assert.Equal(t, "all", re.Row.Fields[0])
	assert.Equal(t, "33", re.Row.Fields[2], "PODS is the cluster-wide total (13+20)")
	assert.Empty(t, re.Row.Fields[1], "per-node cells stay blank")
	assert.Len(t, re.Row.Fields, td.HeaderCount(), "row width must match the header or rendering breaks")
}

func TestAddAllNodesRowIsIdempotent(t *testing.T) {
	td := nodesTable()
	addAllNodesRow(td)
	addAllNodesRow(td)
	addAllNodesRow(td)

	assert.Equal(t, 3, td.RowCount(), "refreshes re-run decorate: the row must not be added twice")
}

func TestAddAllNodesRowSkipsEmptyTable(t *testing.T) {
	h := model1.Header{{Name: "NAME"}, {Name: "PODS"}}
	td := model1.NewTableDataFull(client.NodeGVR, "", h, model1.NewRowEvents(1))
	addAllNodesRow(td)

	assert.Equal(t, 0, td.RowCount(), "no nodes (or no access) -> no `all` row either")
}
