// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func writeKubeconfig(t *testing.T, current string) string {
	t.Helper()
	cfg := api.NewConfig()
	for _, n := range []string{"orbstack", "eks-x"} {
		cfg.Clusters[n] = &api.Cluster{Server: "https://" + n}
		cfg.AuthInfos[n] = &api.AuthInfo{}
		cfg.Contexts[n] = &api.Context{Cluster: n, AuthInfo: n}
	}
	cfg.CurrentContext = current
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, clientcmd.WriteToFile(*cfg, path))
	t.Setenv("KUBECONFIG", path)

	return path
}

func TestRestoreContextPutsBackTheCurrentContext(t *testing.T) {
	path := writeKubeconfig(t, "orbstack")
	prev := currentContext()
	require.Equal(t, "orbstack", prev)

	// an import (aws eks update-kubeconfig and friends) moves current-context to the new cluster
	cfg, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	cfg.CurrentContext = "eks-x"
	require.NoError(t, clientcmd.WriteToFile(*cfg, path))
	require.Equal(t, "eks-x", currentContext())

	require.NoError(t, restoreContext(prev))
	assert.Equal(t, "orbstack", currentContext(), "init must not redirect the next kubectl command at the new cluster")
}

func TestRestoreContextLeavesAMissingContextAlone(t *testing.T) {
	writeKubeconfig(t, "eks-x")
	require.Error(t, restoreContext("deleted-meanwhile"))
	assert.Equal(t, "eks-x", currentContext())
	require.NoError(t, restoreContext(""), "no previous context: nothing to restore")
}

func TestAWSRegionsOrder(t *testing.T) {
	t.Setenv("AWS_REGION", "us-west-2")
	assert.Equal(t, []string{"eu-north-1"}, awsRegions(context.Background(), []string{"eu-north-1"}), "flag wins")
	t.Setenv("PATH", os.TempDir()) // no aws CLI -> falls through to the environment
	assert.Equal(t, []string{"us-west-2"}, awsRegions(context.Background(), nil))
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	assert.Empty(t, awsRegions(context.Background(), nil))
}

func TestImportArgsGiveEKSAReadableContextName(t *testing.T) {
	cli, args, err := importArgs(context.Background(), "aws", "power-lab", []string{"power-lab"}, "eu-north-1")
	require.NoError(t, err)
	assert.Equal(t, "aws", cli)
	assert.Equal(t, []string{"eks", "update-kubeconfig", "--name", "power-lab", "--region", "eu-north-1", "--alias", "power-lab"}, args)
}

func TestInitRejectsUnknownProvider(t *testing.T) {
	err := InitClusters(context.Background(), os.Stderr, InitOptions{Providers: []string{"oracle"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}
