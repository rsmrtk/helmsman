// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package client_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rsmrtk/helmsman/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func writeKubeconfig(t *testing.T, path string, names ...string) {
	t.Helper()
	body := "apiVersion: v1\nkind: Config\nclusters:\n"
	ctxs := "contexts:\n"
	for _, n := range names {
		body += "- name: " + n + "\n  cluster: {server: 'https://127.0.0.1:1'}\n"
		ctxs += "- name: " + n + "\n  context: {cluster: " + n + ", user: u}\n"
	}
	body += ctxs + "users:\n- name: u\n  user: {token: x}\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

// A cluster added to the kubeconfig after helmsman started must show up in the
// context list and be usable, without a restart.
func TestContextsSeeClustersAddedLater(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	writeKubeconfig(t, path, "first")

	flags := genericclioptions.NewConfigFlags(true)
	flags.KubeConfig = &path
	cfg := client.NewConfig(flags)

	cc, err := cfg.Contexts()
	require.NoError(t, err)
	assert.Len(t, cc, 1)

	writeKubeconfig(t, path, "first", "added-later")
	cc, err = cfg.Contexts()
	require.NoError(t, err)
	assert.Len(t, cc, 2)
	assert.Contains(t, cc, "added-later")

	_, err = cfg.GetContext("added-later")
	assert.NoError(t, err, "a new context can be looked up")
}
