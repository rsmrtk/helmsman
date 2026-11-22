// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func writeCfg(t *testing.T, path string, mut func(*api.Config)) {
	t.Helper()
	cfg := api.NewConfig()
	mut(cfg)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, clientcmd.WriteToFile(*cfg, path))
}

func oneCluster(name, server string) func(*api.Config) {
	return func(c *api.Config) {
		c.Clusters[name] = &api.Cluster{Server: server}
		c.AuthInfos[name] = &api.AuthInfo{Token: "tok-" + name}
		c.Contexts[name] = &api.Context{Cluster: name, AuthInfo: name}
		c.CurrentContext = name
	}
}

func TestIsKubeconfigName(t *testing.T) {
	yes := [][2]string{{"kubeconfig", "x"}, {"kubeconfig-eks", "build"}, {"kubeconfig.yaml", "x"}, {"prod.kubeconfig", "x"}, {"eks-kubeconfig", "x"}, {"config", ".kube"}, {"KUBECONFIG", "x"}}
	no := [][2]string{{"config", "src"}, {"config.yaml", ".kube"}, {"main.go", "x"}, {"kubectl", "x"}, {"my-config", "x"}}
	for _, c := range yes {
		assert.True(t, isKubeconfigName(c[0], c[1]), "%v", c)
	}
	for _, c := range no {
		assert.False(t, isKubeconfigName(c[0], c[1]), "%v", c)
	}
}

func TestFindProjectKubeconfigs(t *testing.T) {
	repo := t.TempDir()
	writeCfg(t, filepath.Join(repo, "build", "kubeconfig-eks"), oneCluster("eks", "https://eks"))
	writeCfg(t, filepath.Join(repo, "kubeconfig.yaml"), oneCluster("top", "https://top"))
	writeCfg(t, filepath.Join(repo, "a", "b", "c", "d", "kubeconfig"), oneCluster("too-deep", "https://deep"))
	writeCfg(t, filepath.Join(repo, "node_modules", "x", "kubeconfig"), oneCluster("skipped", "https://skip"))
	writeCfg(t, filepath.Join(repo, ".git", "kubeconfig"), oneCluster("git", "https://git"))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "kubeconfig-notes"), []byte("not yaml: [oops"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "kubeconfig-empty"), nil, 0o600))
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "elsewhere"))

	got := findProjectKubeconfigs(repo, true)
	assert.ElementsMatch(t, []string{filepath.Join(repo, "build", "kubeconfig-eks"), filepath.Join(repo, "kubeconfig.yaml")}, got,
		"found within three levels; vendored, hidden, too deep and unparsable files are ignored")

	// outside a repository only the directory itself is searched
	assert.Equal(t, []string{filepath.Join(repo, "kubeconfig.yaml")}, findProjectKubeconfigs(repo, false))
}

func TestFindSkipsTheDefaultKubeconfigItself(t *testing.T) {
	repo := t.TempDir()
	self := filepath.Join(repo, ".kube", "config")
	writeCfg(t, self, oneCluster("mine", "https://mine"))
	t.Setenv("KUBECONFIG", self)
	assert.Empty(t, findProjectKubeconfigs(repo, true), "the file helmsman already reads is not 'imported' into itself")
}

func TestMergeAddsContextsAndKeepsEverythingElse(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "config")
	src := filepath.Join(dir, "proj", "kubeconfig")
	writeCfg(t, dst, oneCluster("orbstack", "https://orb"))
	writeCfg(t, src, oneCluster("eks-power-lab", "https://eks"))

	res, err := mergeKubeconfig(src, dst, "power-lab", false)
	require.NoError(t, err)
	assert.Equal(t, []string{"eks-power-lab"}, res.added)

	cfg, err := clientcmd.LoadFromFile(dst)
	require.NoError(t, err)
	assert.Contains(t, cfg.Contexts, "orbstack", "what was there stays")
	assert.Contains(t, cfg.Contexts, "eks-power-lab")
	assert.Equal(t, "orbstack", cfg.CurrentContext, "the current context never moves")
	assert.Equal(t, "https://eks", cfg.Clusters["eks-power-lab"].Server)
	assert.Equal(t, "tok-eks-power-lab", cfg.AuthInfos["eks-power-lab"].Token)
}

func TestMergeIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dst, src := filepath.Join(dir, "config"), filepath.Join(dir, "kubeconfig")
	writeCfg(t, dst, oneCluster("a", "https://a"))
	writeCfg(t, src, oneCluster("b", "https://b"))

	first, err := mergeKubeconfig(src, dst, "p", false)
	require.NoError(t, err)
	require.Equal(t, []string{"b"}, first.added)
	before, _ := os.ReadFile(dst)

	second, err := mergeKubeconfig(src, dst, "p", false)
	require.NoError(t, err)
	assert.Empty(t, second.added, "a second run adds nothing")
	assert.Equal(t, []string{"b"}, second.present)
	after, _ := os.ReadFile(dst)
	assert.Equal(t, string(before), string(after), "and does not touch the file")
}

func TestMergeRenamesWhenTheNameBelongsToAnotherCluster(t *testing.T) {
	dir := t.TempDir()
	dst, src := filepath.Join(dir, "config"), filepath.Join(dir, "kubeconfig")
	writeCfg(t, dst, oneCluster("dev", "https://one"))
	writeCfg(t, src, oneCluster("dev", "https://two")) // same name, different cluster

	res, err := mergeKubeconfig(src, dst, "myrepo", false)
	require.NoError(t, err)
	assert.Equal(t, []string{"myrepo-dev"}, res.added, "the existing context is never overwritten")

	cfg, _ := clientcmd.LoadFromFile(dst)
	assert.Equal(t, "https://one", cfg.Clusters[cfg.Contexts["dev"].Cluster].Server)
	assert.Equal(t, "https://two", cfg.Clusters[cfg.Contexts["myrepo-dev"].Cluster].Server)
	assert.Equal(t, "tok-dev", cfg.AuthInfos[cfg.Contexts["dev"].AuthInfo].Token)

	again, err := mergeKubeconfig(src, dst, "myrepo", false)
	require.NoError(t, err)
	assert.Empty(t, again.added, "the renamed copy is recognized the next time")
	assert.Equal(t, []string{"myrepo-dev"}, again.present)
}

func TestMergeDryRunWritesNothingAndMissingTargetIsCreated(t *testing.T) {
	dir := t.TempDir()
	dst, src := filepath.Join(dir, "new", ".kube", "config"), filepath.Join(dir, "kubeconfig")
	writeCfg(t, src, oneCluster("x", "https://x"))

	res, err := mergeKubeconfig(src, dst, "p", true)
	require.NoError(t, err)
	assert.Equal(t, []string{"x"}, res.added)
	_, statErr := os.Stat(dst)
	assert.True(t, os.IsNotExist(statErr), "a dry run creates nothing")

	_, err = mergeKubeconfig(src, dst, "p", false)
	require.NoError(t, err)
	cfg, err := clientcmd.LoadFromFile(dst)
	require.NoError(t, err)
	assert.Contains(t, cfg.Contexts, "x", "a missing kubeconfig, and its directory, are created")
}

func TestMergeMakesRelativeCertificatePathsAbsolute(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "config")
	proj := filepath.Join(dir, "proj")
	src := filepath.Join(proj, "kubeconfig")
	writeCfg(t, src, func(c *api.Config) {
		oneCluster("c", "https://c")(c)
		c.Clusters["c"].CertificateAuthority = "pki/ca.crt" // relative to the file
	})
	_, err := mergeKubeconfig(src, dst, "p", false)
	require.NoError(t, err)
	cfg, _ := clientcmd.LoadFromFile(dst)
	assert.Equal(t, filepath.Join(proj, "pki", "ca.crt"), cfg.Clusters["c"].CertificateAuthority,
		"the copy no longer sits next to the certificate, so the path must not stay relative")
}

func TestMergeRejectsNonKubeconfigs(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "kubeconfig")
	require.NoError(t, os.WriteFile(bad, []byte("{{ not yaml"), 0o600))
	_, err := mergeKubeconfig(bad, filepath.Join(dir, "config"), "p", false)
	require.Error(t, err)

	empty := filepath.Join(dir, "kubeconfig-empty")
	writeCfg(t, empty, func(*api.Config) {})
	_, err = mergeKubeconfig(empty, filepath.Join(dir, "config"), "p", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no contexts")
}

func TestLocalToolContextNames(t *testing.T) {
	byTitle := map[string]localTool{}
	for _, tl := range localTools {
		byTitle[tl.title] = tl
	}
	assert.Equal(t, "kind-demo", byTitle["kind"].context("demo"))
	assert.Equal(t, "k3d-lab", byTitle["k3d"].context("lab"))
	assert.Equal(t, "mini", byTitle["minikube"].context("mini"))
	cli, args := byTitle["kind"].importCmd("demo")
	assert.Equal(t, "kind", cli)
	assert.Equal(t, []string{"export", "kubeconfig", "--name", "demo"}, args)
	_, args = byTitle["k3d"].importCmd("lab")
	assert.Contains(t, args, "--kubeconfig-switch-context=false", "a k3d import must not change the current context")
}

func TestNamesFromJSONAndLines(t *testing.T) {
	names, err := namesFromJSON([]byte(`[{"name":"a","x":1},{"name":""},{"other":"b"},{"name":"c"}]`), "name")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "c"}, names)
	_, err = namesFromJSON([]byte("nope"), "name")
	require.Error(t, err)
	assert.Equal(t, []string{"a", "b"}, nonEmptyLines("a\n\n  b  \n"))
}

func TestInitLocalReportsNothingFoundWithAHint(t *testing.T) {
	repo := t.TempDir()
	t.Chdir(repo)
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "config"))
	onlyGit(t) // no kind, k3d or minikube
	var b safeBuf
	ui := newInitUI(&b, true)
	imp, prob := initLocal(t.Context(), ui, InitOptions{})
	assert.Zero(t, imp)
	assert.Zero(t, prob)
	assert.Contains(t, b.String(), "no kubeconfig files in this project")
	assert.Contains(t, b.String(), "kind create cluster")
}

func TestInitLocalImportsAProjectKubeconfigEndToEnd(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, exec.CommandContext(t.Context(), "git", "-C", repo, "init", "-q").Run(), "a project is a git repository")
	writeCfg(t, filepath.Join(repo, "build", "kubeconfig-eks"), oneCluster("eks-lab", "https://eks"))
	home := filepath.Join(t.TempDir(), "config")
	writeCfg(t, home, oneCluster("orbstack", "https://orb"))
	t.Chdir(repo)
	t.Setenv("KUBECONFIG", home)
	onlyGit(t)

	var b safeBuf
	imp, prob := initLocal(t.Context(), newInitUI(&b, true), InitOptions{})
	assert.Equal(t, 1, imp)
	assert.Zero(t, prob)
	assert.Contains(t, b.String(), "build/kubeconfig-eks")
	cfg, _ := clientcmd.LoadFromFile(home)
	assert.Contains(t, cfg.Contexts, "eks-lab")
	assert.Equal(t, "orbstack", cfg.CurrentContext)

	b.Reset()
	imp, _ = initLocal(t.Context(), newInitUI(&b, true), InitOptions{})
	assert.Zero(t, imp, "running it again imports nothing")
	assert.Contains(t, b.String(), "already in your kubeconfig")
}

// safeBuf is a bytes.Buffer, kept in its own type so the tests above read the same as the others.
type safeBuf struct{ bytes.Buffer }

func TestOutsideARepositoryOnlyTheCurrentDirectoryIsSearched(t *testing.T) {
	dir := t.TempDir() // not a git repository
	writeCfg(t, filepath.Join(dir, "build", "kubeconfig-eks"), oneCluster("deep", "https://deep"))
	t.Chdir(dir)
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "config"))
	onlyGit(t)
	var b safeBuf
	imp, _ := initLocal(t.Context(), newInitUI(&b, true), InitOptions{})
	assert.Zero(t, imp, "scanning below an arbitrary directory (a home directory, say) would be slow and surprising")
}

// onlyGit makes PATH contain git and nothing else, so the tests do not depend on kind, k3d or minikube being installed.
func onlyGit(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if g, err := exec.LookPath("git"); err == nil {
		require.NoError(t, os.Symlink(g, filepath.Join(dir, "git")))
	}
	t.Setenv("PATH", dir)
}
