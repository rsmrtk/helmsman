// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func TestFindNerdFont(t *testing.T) {
	dir := t.TempDir()
	assert.Empty(t, findNerdFont([]string{dir}), "an empty directory has none")
	assert.Empty(t, findNerdFont([]string{filepath.Join(dir, "missing")}), "a missing directory is not an error")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "Inter-Regular.ttf"), nil, 0o600))
	assert.Empty(t, findNerdFont([]string{dir}), "an ordinary font is not a Nerd Font")

	sub := filepath.Join(dir, "deep", "er")
	require.NoError(t, os.MkdirAll(sub, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "JetBrainsMonoNerdFont-Regular.ttf"), nil, 0o600))
	assert.Equal(t, filepath.Join(sub, "JetBrainsMonoNerdFont-Regular.ttf"), findNerdFont([]string{dir}), "found in a subdirectory")

	other := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(other, "x NERD font.OTF"), nil, 0o600))
	assert.NotEmpty(t, findNerdFont([]string{filepath.Join(dir, "none"), other}), "case-insensitive, and later directories are searched")
}

func kubeconfigWith(t *testing.T, contexts ...string) {
	t.Helper()
	cfg := api.NewConfig()
	for _, n := range contexts {
		cfg.Clusters[n] = &api.Cluster{Server: "https://127.0.0.1:1"} // nothing listens there
		cfg.AuthInfos[n] = &api.AuthInfo{}
		cfg.Contexts[n] = &api.Context{Cluster: n, AuthInfo: n}
	}
	if len(contexts) > 0 {
		cfg.CurrentContext = contexts[0]
	}
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, clientcmd.WriteToFile(*cfg, path))
	t.Setenv("KUBECONFIG", path)
}

func TestKubeChecksNoContexts(t *testing.T) {
	kubeconfigWith(t)
	cs := kubeChecks(context.Background())
	require.Len(t, cs, 1)
	assert.Equal(t, checkFail, cs[0].status)
	assert.Contains(t, cs[0].hint, "helmsman init", "an empty kubeconfig points at init")
}

func TestKubeChecksUnreachableClusterIsAWarningNotAFailure(t *testing.T) {
	kubeconfigWith(t, "dead")
	cs := kubeChecks(context.Background())
	require.Len(t, cs, 3)
	assert.Equal(t, checkOK, cs[0].status)
	assert.Contains(t, cs[1].detail, "dead")
	assert.Equal(t, checkWarn, cs[2].status, "a cluster that is down is not a helmsman problem")
	assert.Contains(t, cs[2].hint, "login")
}

func TestKubeChecksNoCurrentContext(t *testing.T) {
	kubeconfigWith(t, "a")
	cfg, err := clientcmd.LoadFromFile(os.Getenv("KUBECONFIG"))
	require.NoError(t, err)
	cfg.CurrentContext = ""
	require.NoError(t, clientcmd.WriteToFile(*cfg, os.Getenv("KUBECONFIG")))
	cs := kubeChecks(context.Background())
	assert.Equal(t, checkWarn, cs[len(cs)-1].status)
}

func TestDoctorOutputAndExitCode(t *testing.T) {
	kubeconfigWith(t) // no clusters: a failure
	var b bytes.Buffer
	err := Doctor(context.Background(), &b, DoctorOptions{NoCloud: true})
	require.Error(t, err, "a failed check makes the command exit non-zero")
	out := b.String()
	assert.Contains(t, out, "helmsman doctor")
	assert.Contains(t, out, "Kubernetes")
	assert.Contains(t, out, "✗ contexts")
	assert.Contains(t, out, "problem(s) to fix")
	assert.NotContains(t, out, "\x1b", "plain text off a terminal")

	kubeconfigWith(t, "dead")
	b.Reset()
	require.NoError(t, Doctor(context.Background(), &b, DoctorOptions{NoCloud: true}), "warnings alone do not fail")
	assert.Contains(t, b.String(), "warning(s)")
}

func TestDoctorGroupsAreHeadedOnce(t *testing.T) {
	kubeconfigWith(t, "dead")
	var b bytes.Buffer
	_ = Doctor(context.Background(), &b, DoctorOptions{NoCloud: true})
	assert.Equal(t, 1, bytes.Count(b.Bytes(), []byte("  Kubernetes\n")))
}

func TestFontCheckTrustsTerminalsWithABuiltInSymbolFont(t *testing.T) {
	t.Setenv("KITTY_WINDOW_ID", "1")
	assert.Equal(t, checkOK, fontCheck().status)
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("WEZTERM_PANE", "1")
	assert.Equal(t, checkOK, fontCheck().status)
}

func TestTermProgramNaming(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	assert.Equal(t, " (ghostty)", termProgram())
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("KITTY_WINDOW_ID", "3")
	assert.Equal(t, " (kitty)", termProgram())
}
