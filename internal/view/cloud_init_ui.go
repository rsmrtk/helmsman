// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/mattn/go-isatty"
)

// The look of `helmsman init`: one block per provider, a status mark per line, errors reduced to what happened and
// what to do about it. Color is used only on a real terminal and never when NO_COLOR is set, so pipes and logs stay
// clean text.

//nolint:gosec // ANSI escape sequences, not credentials
const (
	cReset = "\x1b[0m"
	cBold  = "\x1b[1m"
	cDim   = "\x1b[2m"
	cGreen = "\x1b[38;2;71;186;148m"  // healthy: the same green as Running pods
	cRed   = "\x1b[38;2;235;111;146m" // broken
	cAmber = "\x1b[38;2;246;193;119m" // skipped / attention
	cBlue  = "\x1b[38;2;146;179;244m" // theme blue: headings and provider labels
	cCmd   = "\x1b[38;2;196;167;231m" // commands the user can copy
)

type initUI struct {
	w       io.Writer
	color   bool
	anim    bool         // animated logos (a terminal wide enough only)
	mu      sync.Mutex   // the animation goroutine and the main flow both write to w
	capture bool         // while a logo animates, lines the work prints are held in buf
	buf     bytes.Buffer // ... and written under the finished logo
	status  string       // the line under the animated logo
	scale   int          // size of the heading glyph: 1 is normal, 2 and more use the text sizing protocol
	present int          // clusters found that were already in the kubeconfig
}

func newInitUI(w io.Writer, noAnim bool) *initUI {
	u := &initUI{w: w}
	if f, ok := w.(*os.File); ok && os.Getenv("NO_COLOR") == "" {
		u.color = isTerminal(f)
	}
	u.anim = animationAllowed(w, noAnim)
	u.scale = 1
	if u.anim && !iconsPlain() {
		u.scale = iconScale(w)
	}

	return u
}

func isTerminal(f *os.File) bool { return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd()) }

func (u *initUI) paint(c, s string) string {
	if !u.color || s == "" {
		return s
	}

	return c + s + cReset
}

func (u *initUI) printf(format string, a ...any) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.capture {
		fmt.Fprintf(&u.buf, format, a...)

		return
	}
	fmt.Fprintf(u.w, format, a...)
}

func (u *initUI) header(dry bool) {
	title := "helmsman init"
	sub := "import your clusters into the kubeconfig"
	if dry {
		sub += " (dry run: nothing will be changed)"
	}
	u.printf("\n  %s  %s\n\n", u.paint(cBold+cBlue, title), u.paint(cDim, sub))
}

// provider opens a block. label is padded before coloring so columns stay aligned with or without ANSI codes.
func (u *initUI) provider(name string) {
	u.printf("  %s\n", u.paint(cBold+cBlue, name))
}

func (u *initUI) ok(what, detail string) {
	u.printf("    %s %s  %s\n", u.paint(cGreen, "✓"), u.paint(cBold, padLabel(what)), detail)
}

func (u *initUI) skip(what, detail string) {
	u.printf("    %s %s  %s\n", u.paint(cAmber, "–"), u.paint(cBold, padLabel(what)), u.paint(cAmber, detail))
}

func (u *initUI) fail(what, detail string) {
	u.printf("    %s %s  %s\n", u.paint(cRed, "✗"), u.paint(cBold, padLabel(what)), u.paint(cRed, detail))
}

func (u *initUI) added(name string) {
	u.printf("    %s %s  %s\n", u.paint(cGreen, "+"), u.paint(cBold, padLabel(name)), u.paint(cDim, "added to the kubeconfig"))
}

func (u *initUI) would(name, command string) {
	u.printf("    %s %s  %s\n", u.paint(cBlue, "·"), u.paint(cBold, padLabel(name)), u.paint(cDim, "would run: ")+u.paint(cCmd, command))
}

// hint is what to do about a problem. A command (after "run: ") goes on its own line, highlighted, so it can be copied.
// wouldAdd is the dry-run line for something that is added without running a command.
func (u *initUI) wouldAdd(name, what string) {
	u.printf("    %s %s  %s\n", u.paint(cBlue, "·"), u.paint(cBold, padLabel(name)), u.paint(cDim, "would "+what))
}

func (u *initUI) hint(text string) {
	if text == "" {
		return
	}
	text, cmd, hasCmd := strings.Cut(text, "run: ")
	u.printf("      %s %s\n", u.paint(cDim, "↳"), u.paint(cDim, strings.TrimSpace(text)))
	if hasCmd {
		u.printf("        %s\n", u.paint(cCmd, strings.TrimSpace(cmd)))
	}
}

func (u *initUI) blank() { u.printf("\n") }

func (u *initUI) summary(imported, problems int, dry bool, current string) {
	switch {
	case imported == 0 && problems == 0 && u.present > 0 && !dry:
		u.printf("  %s %s\n    %s\n\n", u.paint(cGreen, "✓"), u.paint(cBold, "Everything is already in your kubeconfig."),
			u.paint(cDim, fmt.Sprintf("%d cluster(s) found, nothing to add.", u.present)))
	case dry:
		u.printf("  %s\n\n", u.paint(cDim, "Dry run finished. Nothing was changed."))
	case imported > 0:
		u.printf("  %s %s\n", u.paint(cGreen, "✓"), u.paint(cBold, fmt.Sprintf("%d cluster(s) imported.", imported)))
		u.printf("    %s\n\n", u.paint(cDim, "Your current context is unchanged ("+firstNonEmpty(current, "none")+"). Start "+"`helmsman`"+" and pick them."))
	case problems > 0:
		msg := fmt.Sprintf("Nothing imported: %d %s above. Fix %s and run init again.",
			problems, noun(problems, "problem", "problems"), noun(problems, "it", "them"))
		u.printf("  %s %s\n\n", u.paint(cAmber, "–"), u.paint(cBold, msg))
	default:
		msg := "No clusters found. Sign in to a cloud (aws login, gcloud auth login, az login) or start a local one, then run init again."
		u.printf("  %s %s\n\n", u.paint(cAmber, "–"), u.paint(cBold, msg))
	}
}

func noun(n int, one, many string) string {
	if n == 1 {
		return one
	}

	return many
}

// labelWidth is the width of the label column (account, us-east-1, EKS ...), so the details line up.
const labelWidth = 13

func padLabel(s string) string {
	if r := []rune(s); len(r) < labelWidth {
		return s + strings.Repeat(" ", labelWidth-len(r))
	}

	return s
}

// ---------------------------------------------------------------------------------------------------------------------
// Turning a raw CLI error into "what happened" + "what to do".

var (
	reARN      = regexp.MustCompile(`arn:aws:sts::(\d+):assumed-role/([^/]+)/`)
	reARNUser  = regexp.MustCompile(`arn:aws:(?:iam|sts)::(\d+):(?:user|root)(?:/(\S+))?`)
	reProject  = regexp.MustCompile(`project (\S+)`)
	reRegionIn = regexp.MustCompile(`region (\S+)`)
)

// identityOf shortens whoami's text: an assumed-role ARN becomes "role @ account", and the trailing region/project stays.
func identityOf(prov, who string) (identity, scope string) {
	main, rest, _ := strings.Cut(who, "  ")
	if prov == provAWS {
		if m := reARN.FindStringSubmatch(main); m != nil {
			main = m[2] + " @ " + m[1]
		} else if m := reARNUser.FindStringSubmatch(main); m != nil {
			main = firstNonEmpty(m[2], "root") + " @ " + m[1]
		}
		if m := reRegionIn.FindStringSubmatch(rest); m != nil {
			rest = "region " + m[1]
		}
	}

	return main, strings.TrimSpace(rest)
}

// explain says, in a line, what a provider CLI error means and what to do. project fills in the hint.
func explain(prov, raw, project string) (reason, hint string) {
	low := strings.ToLower(raw)
	switch {
	case strings.Contains(low, "is not installed"):
		cli := cliOf(prov)

		return cli + " CLI is not installed", map[string]string{
			provAWS: "install it: brew install awscli", provGCP: "install it: brew install --cask gcloud-cli",
			provAzure: "install it: brew install azure-cli",
		}[prov]
	case prov == provAWS && (strings.Contains(low, "accessdenied") || strings.Contains(low, "unauthorizedoperation")):
		return "access denied: cannot list EKS clusters",
			"this region may be blocked for your account, or the role lacks eks:ListClusters. Try another region: run: helmsman init aws --region <region>"
	case prov == provAWS && (strings.Contains(low, "expired") || strings.Contains(low, "unable to locate credentials") || strings.Contains(low, "aws login")):
		return "AWS session expired or missing", "sign in again, then run init again: run: aws login"
	case prov == provAWS && strings.Contains(low, "could not connect"):
		return "cannot reach AWS", "check your network or VPN and try again"
	case prov == provGCP && (strings.Contains(low, "has not been used") || strings.Contains(low, "is disabled") || strings.Contains(low, "service_disabled")):
		if project == "" {
			project = "<project>"
		}

		return "Kubernetes Engine API is disabled in project " + project,
			"enable it (takes about a minute), then run init again: run: gcloud services enable container.googleapis.com --project " + project
	case prov == provGCP && (strings.Contains(low, "reauth") || strings.Contains(low, "gcloud auth login") || strings.Contains(low, "not signed in")):
		return "not signed in to Google Cloud", "sign in, then run init again: run: gcloud auth login"
	case prov == provAzure && (strings.Contains(low, "az login") || strings.Contains(low, "not signed in")):
		return "not signed in to Azure", "sign in, then run init again: run: az login"
	case strings.Contains(low, "403") || strings.Contains(low, "permission"):
		return "permission denied", "ask for a role that can list Kubernetes clusters"
	}

	return truncate(strings.TrimSpace(raw), 110), ""
}
