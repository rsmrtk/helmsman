// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/clientcmd"
)

// DoctorOptions controls `helmsman doctor`.
type DoctorOptions struct {
	Version string
	NoCloud bool // skip the cloud CLI sign-in checks (they call out to the providers)
}

// names of checks that are built in more than one place
const (
	nameCluster = "Kubernetes/cluster"
	nameIcons   = "Terminal/icons"
)

const (
	checkOK = iota
	checkWarn
	checkFail
)

type doctorCheck struct {
	status int
	name   string
	detail string
	hint   string
}

// Doctor looks at everything helmsman depends on and says what to fix: the kubeconfig and the cluster behind the current
// context, the terminal (colors, animation, the big heading glyph), a Nerd Font for the icons, and the cloud CLIs
// that `helmsman init` uses. It changes nothing. A failed check makes the command exit non-zero.
func Doctor(ctx context.Context, w io.Writer, o DoctorOptions) error {
	ui := newInitUI(w, true) // doctor prints plain checks: no animation
	ui.printf("\n  %s  %s\n\n", ui.paint(cBold+cBlue, "helmsman doctor"), ui.paint(cDim, "what helmsman needs, and what is missing"))

	var checks []doctorCheck
	checks = append(checks, kubeChecks(ctx)...)
	checks = append(checks, terminalChecks(w)...)
	if !o.NoCloud {
		checks = append(checks, cloudChecks(ctx)...)
	}

	fails, warns := 0, 0
	group := ""
	for _, c := range checks {
		if g, _, _ := strings.Cut(c.name, "/"); g != group {
			group = g
			ui.printf("  %s\n", ui.paint(cBold+cBlue, group))
		}
		label := strings.TrimPrefix(c.name, group+"/")
		switch c.status {
		case checkOK:
			ui.ok(label, c.detail)
		case checkWarn:
			ui.skip(label, c.detail)
			warns++
		default:
			ui.fail(label, c.detail)
			fails++
		}
		ui.hint(c.hint)
	}
	ui.blank()
	switch {
	case fails > 0:
		ui.printf("  %s %s\n\n", ui.paint(cRed, "✗"), ui.paint(cBold, fmt.Sprintf("%d problem(s) to fix, %d warning(s).", fails, warns)))

		return fmt.Errorf("%d check(s) failed", fails)
	case warns > 0:
		ui.printf("  %s %s\n\n", ui.paint(cAmber, "–"), ui.paint(cBold, fmt.Sprintf("Works, with %d warning(s).", warns)))
	default:
		ui.printf("  %s %s\n\n", ui.paint(cGreen, "✓"), ui.paint(cBold, "Everything looks good."))
	}

	return nil
}

func kubeChecks(ctx context.Context) []doctorCheck {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg, err := rules.Load()
	if err != nil {
		return []doctorCheck{{checkFail, "Kubernetes/kubeconfig", "cannot read it: " + truncate(err.Error(), 100), "check the KUBECONFIG variable and the file permissions"}}
	}
	var out []doctorCheck
	switch n := len(cfg.Contexts); {
	case n == 0:
		out = append(out, doctorCheck{checkFail, "Kubernetes/contexts", "the kubeconfig has no clusters",
			"import your cloud clusters: run: helmsman init"})

		return out
	default:
		where := firstNonEmpty(strings.Join(rules.Precedence, ", "), "kubeconfig")
		out = append(out, doctorCheck{checkOK, "Kubernetes/contexts", fmt.Sprintf("%d in %s", n, where), ""})
	}
	if cfg.CurrentContext == "" {
		hint := "helmsman starts on the cluster picker, so this is fine; kubectl needs one: run: kubectl config use-context <name>"
		out = append(out, doctorCheck{checkWarn, "Kubernetes/current", "no current context", hint})

		return out
	}
	out = append(out,
		doctorCheck{checkOK, "Kubernetes/current", cfg.CurrentContext, ""},
		reachCheck(ctx, rules, cfg.CurrentContext))

	return out
}

// reachCheck asks the cluster behind a context for its version, with a short timeout.
func reachCheck(ctx context.Context, rules *clientcmd.ClientConfigLoadingRules, name string) doctorCheck {
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{CurrentContext: name})
	rc, err := cc.ClientConfig()
	if err != nil {
		return doctorCheck{checkFail, nameCluster, "cannot build a client for " + name + ": " + truncate(err.Error(), 80), ""}
	}
	rc.Timeout = 5 * time.Second
	dc, err := discovery.NewDiscoveryClientForConfig(rc)
	if err != nil {
		return doctorCheck{checkFail, nameCluster, truncate(err.Error(), 100), ""}
	}
	type result struct {
		v   string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := dc.ServerVersion()
		if err != nil {
			ch <- result{err: err}

			return
		}
		ch <- result{v: v.GitVersion}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return doctorCheck{checkWarn, nameCluster, "not reachable: " + truncate(r.err.Error(), 90),
				"is the cluster up, your IP allowed and your cloud session valid? (aws login / gcloud auth login / az login)"}
		}

		return doctorCheck{checkOK, nameCluster, "reachable, server " + r.v, ""}
	case <-ctx.Done():
		return doctorCheck{checkWarn, nameCluster, "check canceled", ""}
	}
}

func terminalChecks(w io.Writer) []doctorCheck {
	var out []doctorCheck
	f, isFile := w.(*os.File)
	if !isFile || !isTerminal(f) {
		return append(out, doctorCheck{checkWarn, "Terminal/tty", "output is not a terminal", "run it in a terminal to check colors and animation"})
	}
	out = append(out, doctorCheck{checkOK, "Terminal/tty", firstNonEmpty(os.Getenv("TERM"), "?") + termProgram(), ""})
	if ct := os.Getenv("COLORTERM"); ct == "truecolor" || ct == "24bit" {
		out = append(out, doctorCheck{checkOK, "Terminal/colors", "true color", ""})
	} else {
		out = append(out, doctorCheck{checkWarn, "Terminal/colors", "true color is not advertised (COLORTERM=" + ct + ")",
			"themes and the init animations look best with true color; most modern terminals have it"})
	}
	if os.Getenv("NO_COLOR") != "" {
		out = append(out, doctorCheck{checkWarn, "Terminal/NO_COLOR", "set: colors are off", ""})
	}
	if os.Getenv("HELMSMAN_NO_ANIM") != "" {
		out = append(out, doctorCheck{checkWarn, "Terminal/animation", "turned off by HELMSMAN_NO_ANIM", ""})
	}
	if iconScale(w) > 1 {
		out = append(out, doctorCheck{checkOK, "Terminal/big glyph", "text sizing supported: the heading glyph is drawn bigger", ""})
	} else {
		out = append(out, doctorCheck{checkWarn, "Terminal/big glyph", "not supported here: the heading glyph is normal size",
			"kitty (and some others) can draw it bigger; set HELMSMAN_ICON_SCALE=2 to force it"})
	}
	out = append(out, fontCheck())

	return out
}

func termProgram() string {
	if p := os.Getenv("TERM_PROGRAM"); p != "" {
		return " (" + p + ")"
	}
	switch {
	case os.Getenv("KITTY_WINDOW_ID") != "":
		return " (kitty)"
	case os.Getenv("WEZTERM_PANE") != "":
		return " (WezTerm)"
	}

	return ""
}

// fontCheck looks for a Nerd Font, which carries the icons helmsman draws. Terminals that ship their own symbol font
// (kitty, WezTerm) need none installed.
func fontCheck() doctorCheck {
	switch {
	case os.Getenv("KITTY_WINDOW_ID") != "":
		return doctorCheck{checkOK, nameIcons, "kitty draws the icons with its built-in symbol font", ""}
	case os.Getenv("WEZTERM_PANE") != "":
		return doctorCheck{checkOK, nameIcons, "WezTerm draws the icons with its built-in symbol font", ""}
	}
	if p := findNerdFont(fontDirs()); p != "" {
		return doctorCheck{checkOK, nameIcons, "Nerd Font installed (" + filepath.Base(p) + ")", "make sure your terminal uses it"}
	}

	return doctorCheck{checkWarn, nameIcons, "no Nerd Font found: icons may show as empty squares",
		"install one: run: brew install --cask font-jetbrains-mono-nerd-font   (or set HELMSMAN_ICONS=plain for text)"}
}

func fontDirs() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return []string{filepath.Join(home, "Library", "Fonts"), "/Library/Fonts"}
	case "windows":
		return []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts"), `C:\Windows\Fonts`}
	default:
		return []string{filepath.Join(home, ".local", "share", "fonts"), filepath.Join(home, ".fonts"), "/usr/share/fonts", "/usr/local/share/fonts"}
	}
}

// findNerdFont returns the first font file under dirs whose name says it is a Nerd Font, or "".
func findNerdFont(dirs []string) string {
	found := ""
	for _, d := range dirs {
		_ = filepath.WalkDir(d, func(p string, e fs.DirEntry, err error) error {
			if err != nil || found != "" {
				return filepath.SkipDir
			}
			if !e.IsDir() {
				n := strings.ToLower(e.Name())
				if strings.Contains(n, "nerd") && (strings.HasSuffix(n, ".ttf") || strings.HasSuffix(n, ".otf")) {
					found = p

					return filepath.SkipAll
				}
			}

			return nil
		})
		if found != "" {
			break
		}
	}

	return found
}

func cloudChecks(ctx context.Context) []doctorCheck {
	var out []doctorCheck
	for _, prov := range []string{provAWS, provGCP, provAzure} {
		name := "Cloud/" + providerTitle[prov]
		cli := cliOf(prov)
		if _, err := exec.LookPath(cli); err != nil {
			out = append(out, doctorCheck{checkWarn, name, cli + " is not installed", "only needed to import " + providerTitle[prov] + " clusters"})

			continue
		}
		who, err := whoami(ctx, prov)
		if err != nil {
			reason, hint := explain(prov, err.Error(), "")
			out = append(out, doctorCheck{checkWarn, name, reason, hint})

			continue
		}
		id, scope := identityOf(prov, who)
		out = append(out, doctorCheck{checkOK, name, id + scopeSuffix(scope), ""})
	}

	return out
}
