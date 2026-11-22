// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// The "local" target of `helmsman init`: clusters that live on this machine or in the project you are standing in.
//
//	kubeconfig files in the current git repository (build/kubeconfig-eks, kubeconfig.yaml, .kube/config ...)
//	kind clusters, k3d clusters, minikube profiles
//
// Each is made visible in the default kubeconfig, so helmsman and kubectl see them from any terminal. Nothing is removed
// and the current context is never changed.

const provLocal = "local"

// the local cluster tools and a flag they share
const (
	cliKind     = "kind"
	cliK3d      = "k3d"
	cliMinikube = "minikube"
	flagName    = "--name"
	argKubeconf = "kubeconfig"
)

const (
	maxRepoDepth    = 3 // how many directory levels to enter when looking for kubeconfig files inside a repository
	maxVisitedFiles = 20000
	maxKubeconfig   = 1 << 20 // a kubeconfig is a few KiB; anything bigger is not one
)

// skipDirs are never entered while looking for kubeconfig files.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".terraform": true, "dist": true, ".venv": true, "venv": true,
	"__pycache__": true, "target": true, ".cache": true, ".idea": true,
}

// initLocal does the local discovery and reports through ui. It returns how many contexts it added and how many problems it hit.
func initLocal(ctx context.Context, ui *initUI, o InitOptions) (imported, problems int) {
	defer ui.blank()
	dst := defaultKubeconfigPath()
	found := false

	// 1. kubeconfig files of the project
	root, inRepo := projectRoot()
	files := step0(ui, "looking for kubeconfig files in "+filepath.Base(root)+"…", func() []string { return findProjectKubeconfigs(root, inRepo) })
	for _, f := range files {
		found = true
		rel, err := filepath.Rel(root, f)
		if err != nil {
			rel = f
		}
		res, err := mergeKubeconfig(f, dst, filepath.Base(root), o.DryRun)
		switch {
		case err != nil:
			ui.fail(rel, truncate(err.Error(), 90))
			problems++
		case len(res.added) == 0 && len(res.present) > 0:
			ui.ok(rel, "already in your kubeconfig ("+strings.Join(res.present, ", ")+")")
			ui.present += len(res.present)
		case o.DryRun:
			ui.wouldAdd(rel, "add context "+strings.Join(res.added, ", "))
		default:
			ui.added(rel + " → " + strings.Join(res.added, ", "))
			imported += len(res.added)
		}
	}

	// 2. local cluster tools
	for _, tool := range localTools {
		if _, err := exec.LookPath(tool.cli); err != nil {
			continue
		}
		names, err := step(ui, "listing "+tool.title+" clusters…", func() ([]string, error) { return tool.list(ctx) })
		if err != nil {
			ui.skip(tool.title, truncate(err.Error(), 90))

			continue
		}
		for _, name := range names {
			found = true
			cname := tool.context(name)
			if contextExists(cname) {
				ui.ok(tool.title+" "+name, "already in your kubeconfig ("+cname+")")
				ui.present++

				continue
			}
			cli, args := tool.importCmd(name)
			if o.DryRun {
				ui.would(tool.title+" "+name, cli+" "+strings.Join(args, " "))

				continue
			}
			if _, err := step(ui, "importing "+name+"…", func() ([]byte, error) { return runCLI(ctx, cli, args...) }); err != nil {
				ui.fail(tool.title+" "+name, truncate(err.Error(), 90))
				problems++

				continue
			}
			ui.added(tool.title + " " + name + " → " + cname)
			imported++
		}
	}
	if !found {
		ui.skip("local", "no kubeconfig files in this project and no kind, k3d or minikube clusters")
		ui.hint("start one (for example: run: kind create cluster) or cd into a project that has a kubeconfig")
	}

	return imported, problems
}

// step0 is step for a function that cannot fail.
func step0[T any](u *initUI, msg string, f func() T) T {
	v, _ := step(u, msg, func() (T, error) { return f(), nil })

	return v
}

// localTool is one way a cluster lives on this machine.
type localTool struct {
	cli, title string
	list       func(ctx context.Context) ([]string, error)
	context    func(name string) string             // the kubeconfig context the tool gives the cluster
	importCmd  func(name string) (string, []string) // the command that adds it to the kubeconfig
}

var localTools = []localTool{
	{
		cli: cliKind, title: cliKind,
		list: func(ctx context.Context) ([]string, error) {
			out, err := runCLI(ctx, cliKind, "get", "clusters")
			if err != nil {
				return nil, err
			}

			return nonEmptyLines(string(out)), nil
		},
		context:   func(n string) string { return "kind-" + n },
		importCmd: func(n string) (string, []string) { return cliKind, []string{"export", argKubeconf, flagName, n} },
	},
	{
		cli: cliK3d, title: cliK3d,
		list: func(ctx context.Context) ([]string, error) {
			out, err := runCLI(ctx, cliK3d, "cluster", "list", "-o", "json")
			if err != nil {
				return nil, err
			}

			return namesFromJSON(out, "name")
		},
		context: func(n string) string { return "k3d-" + n },
		importCmd: func(n string) (string, []string) {
			return cliK3d, []string{argKubeconf, "merge", n, "--kubeconfig-merge-default", "--kubeconfig-switch-context=false"}
		},
	},
	{
		cli: cliMinikube, title: cliMinikube,
		list: func(ctx context.Context) ([]string, error) {
			out, err := runCLI(ctx, cliMinikube, "profile", "list", "-o", "json")
			if err != nil {
				return nil, err
			}
			var doc struct {
				Valid []struct{ Name string } `json:"valid"`
			}
			if err := json.Unmarshal(out, &doc); err != nil {
				return nil, err
			}
			names := make([]string, 0, len(doc.Valid))
			for _, p := range doc.Valid {
				names = append(names, p.Name)
			}

			return names, nil
		},
		context:   func(n string) string { return n },
		importCmd: func(n string) (string, []string) { return cliMinikube, []string{"update-context", "-p", n} },
	},
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}

	return out
}

// namesFromJSON reads a JSON array of objects and returns the string field `key` of each.
func namesFromJSON(b []byte, key string) ([]string, error) {
	var arr []map[string]any
	if err := json.Unmarshal(b, &arr); err != nil {
		return nil, err
	}
	var out []string
	for _, m := range arr {
		if s, ok := m[key].(string); ok && s != "" {
			out = append(out, s)
		}
	}

	return out, nil
}

// ---------------------------------------------------------------------------------------------------------------------

// defaultKubeconfigPath is the file helmsman and kubectl read first: the first path of KUBECONFIG, else ~/.kube/config.
func defaultKubeconfigPath() string {
	return clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
}

func contextExists(name string) bool {
	cfg, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return false
	}
	_, ok := cfg.Contexts[name]

	return ok
}

// projectRoot is the git repository you are standing in, else the current directory. inRepo tells which, because outside
// a repository only the current directory is searched: scanning a whole home directory would be slow and surprising.
func projectRoot() (root string, inRepo bool) {
	wd, err := os.Getwd()
	if err != nil {
		return ".", false
	}
	if out, err := exec.CommandContext(context.Background(), "git", "-C", wd, "rev-parse", "--show-toplevel").Output(); err == nil {
		if r := strings.TrimSpace(string(out)); r != "" {
			return r, true
		}
	}

	return wd, false
}

// isKubeconfigName says whether a file name looks like a kubeconfig. dir is the name of its parent directory.
func isKubeconfigName(name, dir string) bool {
	n := strings.ToLower(name)
	switch {
	case n == "kubeconfig", strings.HasPrefix(n, "kubeconfig-"), strings.HasPrefix(n, "kubeconfig."), strings.HasSuffix(n, ".kubeconfig"),
		strings.HasSuffix(n, "-kubeconfig"):
		return true
	case n == "config" && dir == ".kube":
		return true
	}

	return false
}

// findProjectKubeconfigs returns the kubeconfig files under root. In a repository it looks a few levels deep; elsewhere
// only in root itself. Files that do not parse as a kubeconfig with at least one context are dropped, and so is the
// default kubeconfig itself.
func findProjectKubeconfigs(root string, inRepo bool) []string {
	levels := 0 // how many directory levels below root may be entered: none outside a repository
	if inRepo {
		levels = maxRepoDepth
	}
	self, _ := filepath.Abs(defaultKubeconfigPath())
	var out []string
	visited := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable entry is skipped, not fatal
		}
		visited++
		if visited > maxVisitedFiles {
			return filepath.SkipAll
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			tooDeep := strings.Count(rel, string(filepath.Separator)) >= levels
			if p != root && (skipDirs[d.Name()] || tooDeep) {
				return filepath.SkipDir
			}

			return nil
		}
		if !isKubeconfigName(d.Name(), filepath.Base(filepath.Dir(p))) {
			return nil
		}
		if abs, _ := filepath.Abs(p); abs == self {
			return nil
		}
		if info, err := d.Info(); err != nil || info.Size() == 0 || info.Size() > maxKubeconfig {
			return nil
		}
		if cfg, err := clientcmd.LoadFromFile(p); err == nil && len(cfg.Contexts) > 0 {
			out = append(out, p)
		}

		return nil
	})

	return out
}

// mergeResult says which contexts of a file were added to the target and which were already there.
type mergeResult struct {
	added   []string
	present []string
}

// mergeKubeconfig copies the contexts of the kubeconfig at src into the one at dstPath, leaving everything already there
// untouched and never changing current-context. A context that is already present with the same server is skipped, so
// running it again changes nothing; one whose name is taken by a different cluster is added as <label>-<name>. Relative
// certificate paths are made absolute, since the copy no longer sits next to them. dry reports without writing.
func mergeKubeconfig(src, dstPath, label string, dry bool) (mergeResult, error) {
	var res mergeResult
	in, err := clientcmd.LoadFromFile(src)
	if err != nil {
		return res, fmt.Errorf("not a readable kubeconfig: %w", err)
	}
	if len(in.Contexts) == 0 {
		return res, errors.New("it has no contexts")
	}
	if err := clientcmd.ResolveLocalPaths(in); err != nil {
		return res, fmt.Errorf("cannot resolve its file paths: %w", err)
	}
	dst := api.NewConfig()
	if _, err := os.Stat(dstPath); err == nil {
		if dst, err = clientcmd.LoadFromFile(dstPath); err != nil {
			return res, fmt.Errorf("cannot read %s: %w", dstPath, err)
		}
	}

	names := make([]string, 0, len(in.Contexts))
	for name := range in.Contexts {
		names = append(names, name)
	}
	sort.Strings(names) // a stable order: the same file always reports and merges the same way
	for _, name := range names {
		c := in.Contexts[name]
		cluster, user := in.Clusters[c.Cluster], in.AuthInfos[c.AuthInfo]
		if cluster == nil {
			continue
		}
		target := name
		if existing, ok := dst.Contexts[name]; ok {
			if ec := dst.Clusters[existing.Cluster]; ec != nil && ec.Server == cluster.Server {
				res.present = append(res.present, name)

				continue
			}
			target = label + "-" + name
			if existing, ok := dst.Contexts[target]; ok {
				if ec := dst.Clusters[existing.Cluster]; ec != nil && ec.Server == cluster.Server {
					res.present = append(res.present, target)
				}

				continue
			}
		}
		cname, uname := c.Cluster, c.AuthInfo
		if old := dst.Clusters[cname]; old != nil && !reflect.DeepEqual(old, cluster) {
			cname = target
		}
		dst.Clusters[cname] = cluster
		if user != nil {
			if old := dst.AuthInfos[uname]; old != nil && !reflect.DeepEqual(old, user) {
				uname = target
			}
			dst.AuthInfos[uname] = user
		} else {
			uname = ""
		}
		dst.Contexts[target] = &api.Context{Cluster: cname, AuthInfo: uname, Namespace: c.Namespace}
		res.added = append(res.added, target)
	}
	if dry || len(res.added) == 0 {
		return res, nil
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o750); err != nil {
		return res, err
	}

	return res, clientcmd.WriteToFile(*dst, dstPath)
}
