// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseInitTargets(t *testing.T) {
	everything := []string{"local", "aws", "gcp", "azure"}
	cases := []struct {
		args []string
		want []string
	}{
		{nil, everything},
		{[]string{"all"}, everything},
		{[]string{"ALL"}, everything},
		{[]string{"local"}, []string{"local"}},
		{[]string{"aws"}, []string{"aws"}},
		{[]string{"local", "aws"}, []string{"local", "aws"}},
		{[]string{"aws", "local"}, []string{"local", "aws"}},
		{[]string{"azure", "aws", "gcp"}, []string{"aws", "gcp", "azure"}},
		{[]string{"aws", "aws", "local"}, []string{"local", "aws"}},
		{[]string{"local", "all"}, everything},
	}
	for _, c := range cases {
		got, err := parseInitTargets(c.args)
		require.NoError(t, err, c.args)
		assert.Equal(t, c.want, got, "%v", c.args)
	}
}

func TestParseInitTargetsRejectsUnknownWords(t *testing.T) {
	_, err := parseInitTargets([]string{"aws", "oracle"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown target "oracle"`)
	assert.Contains(t, err.Error(), "all, local, aws, gcp, azure", "the message lists what is valid")
}

func TestInitCommandShape(t *testing.T) {
	c := initCmd()
	assert.Equal(t, "demo", c.Commands()[0].Name(), "demo stays a subcommand")
	require.NoError(t, c.Args(c, []string{"local", "aws"}))
	require.Error(t, c.Args(c, []string{"nope"}))
	assert.NotNil(t, c.PersistentFlags().Lookup("dry-run"))
	assert.NotNil(t, c.PersistentFlags().Lookup("region"))
}
