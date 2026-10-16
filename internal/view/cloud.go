// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const cloudTimeout = 30 * time.Second

var providerNames = map[string]string{
	"aws": "aws", "amazon": "aws",
	"gcp": "gcp", "gcloud": "gcp", "google": "gcp",
	"azure": "azure", "az": "azure",
}

func providerOf(s string) (string, bool) {
	p, ok := providerNames[strings.ToLower(s)]

	return p, ok
}

func runCLI(ctx context.Context, cli string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath(cli); err != nil {
		return nil, fmt.Errorf("%s is not installed", cli)
	}
	ctx, cancel := context.WithTimeout(ctx, cloudTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := cliError(errb.String(), err)

		return nil, errors.New(msg)
	}

	return out.Bytes(), nil
}

// whoami says which account a provider CLI is signed in to, or why it is not.
func whoami(ctx context.Context, provider string) (string, error) {
	switch provider {
	case "aws":
		out, err := runCLI(ctx, "aws", "sts", "get-caller-identity", "--output", "json")
		if err != nil {
			return "", err
		}
		region, _ := runCLI(ctx, "aws", "configure", "get", "region")

		return fmt.Sprintf("%s  region %s", field(mustJSON(out), "Arn"), firstNonEmpty(strings.TrimSpace(string(region)))), nil
	case "gcp":
		acc, err := runCLI(ctx, "gcloud", "config", "get-value", "account")
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(string(acc)) == "" {
			return "", errors.New("not signed in")
		}
		proj, _ := runCLI(ctx, "gcloud", "config", "get-value", "project")

		return fmt.Sprintf("%s  project %s", strings.TrimSpace(string(acc)), firstNonEmpty(strings.TrimSpace(string(proj)))), nil
	default:
		out, err := runCLI(ctx, "az", "account", "show", "-o", "json")
		if err != nil {
			return "", err
		}
		doc := mustJSON(out)

		return fmt.Sprintf("%s  subscription %s", field(doc, "user.name"), field(doc, "name")), nil
	}
}

func mustJSON(b []byte) any {
	rows, _ := parseJSON(b)

	return rows
}

func cloudHome(status map[string]string) string {
	var sb strings.Builder
	sb.WriteString(heading(pad("PROVIDER", 10), "ACCOUNT"))
	for _, p := range []string{"aws", "gcp", "azure"} {
		st := status[p]
		tag := statusTag("Running")
		if strings.HasPrefix(st, "✖") {
			tag = statusTag("CrashLoopBackOff")
		}
		fmt.Fprintf(&sb, "%s●%s %s %s\n", tag, tagReset, pad(p, 8), st)
	}
	sb.WriteString("\n" + tagHeading + "Commands" + tagReset + "\n")
	for _, l := range []string{
		":cloud aws|gcp|azure <service>    list resources",
		"    aws    ec2  s3  eks  rds",
		"    gcp    vm  gke  buckets",
		"    azure  vm  aks  storage  group",
		":cloud login aws|gcp|azure         sign in (opens the provider's own login)",
		":cloud import <cluster>            add an EKS / GKE / AKS cluster to your kubeconfig",
	} {
		sb.WriteString(l + "\n")
	}
	sb.WriteString("\n" + tagDim + "Credentials stay with the provider CLIs; helmsman stores none." + tagReset + "\n")

	return sb.String()
}

// cloudCmd handles ":cloud [provider service | login p | import cluster]".
func (a *App) cloudCmd(args string) {
	f := strings.Fields(args)
	switch {
	case len(f) == 0:
		a.textScreen("Cloud", "accounts", func() string {
			status := map[string]string{}
			names := []string{"aws", "gcp", "azure"}
			res := forEachContext(names, func(ctx context.Context, p string) string {
				if s, err := whoami(ctx, p); err != nil {
					return "✖ " + err.Error()
				} else {
					return s
				}
			})
			for i, p := range names {
				status[p] = res[i]
			}

			return cloudHome(status)
		})
	case f[0] == "login":
		a.cloudLogin(f[1:])
	case f[0] == "import" && len(f) > 1:
		a.cloudImport(f[1])
	case len(f) >= 2:
		p, ok := providerOf(f[0])
		if !ok {
			a.Flash().Errf("unknown provider %q: use aws, gcp or azure", f[0])
			return
		}
		svc, ok := findService(p, f[1])
		if !ok {
			a.Flash().Errf("unknown %s service %q", p, f[1])
			return
		}
		a.textScreen("Cloud", p+" "+svc.Name, func() string {
			ctx, cancel := context.WithTimeout(context.Background(), cloudTimeout)
			defer cancel()
			out, err := runCLI(ctx, cliOf(p), svc.Args...)
			if err != nil {
				return fmt.Sprintf("%s✖ %s%s\n", statusTag("CrashLoopBackOff"), err, tagReset)
			}
			rows, err := parseCloud(svc, out)
			if err != nil {
				return err.Error()
			}

			return cloudText(svc, rows)
		})
	default:
		a.Flash().Errf("use `cloud`, `cloud <aws|gcp|azure> <service>`, `cloud login <provider>` or `cloud import <cluster>`")
	}
}

var loginArgs = map[string][]string{
	"aws":   {"aws", "sso", "login"},
	"gcp":   {"gcloud", "auth", "login"},
	"azure": {"az", "login"},
}

func (a *App) cloudLogin(args []string) {
	if len(args) == 0 {
		a.Flash().Errf("use `cloud login <aws|gcp|azure>`")
		return
	}
	p, ok := providerOf(args[0])
	if !ok {
		a.Flash().Errf("unknown provider %q", args[0])
		return
	}
	cmd := loginArgs[p]
	if _, err := exec.LookPath(cmd[0]); err != nil {
		a.Flash().Errf("%s is not installed", cmd[0])
		return
	}
	// hand the terminal to the provider's own sign-in, then come back
	run(a, &shellOpts{clear: true, binary: cmd[0], args: cmd[1:], banner: "Signing in to " + p})
	a.Flash().Infof("Signed in? Check with :cloud")
}

// importPlan is the command that adds a managed cluster to the kubeconfig.
func importPlan(provider, name string, row []string, region, project string) (cli string, args []string, err error) {
	switch provider {
	case "aws":
		if region == "" {
			return "", nil, errors.New("no default AWS region: run `aws configure set region <region>`")
		}
		return "aws", []string{"eks", "update-kubeconfig", "--name", name, "--region", region}, nil
	case "gcp":
		if len(row) < 3 {
			return "", nil, errors.New("cluster location unknown")
		}
		a := []string{"container", "clusters", "get-credentials", name, "--location", row[2]}
		if project != "" {
			a = append(a, "--project", project)
		}
		return "gcloud", a, nil
	default:
		if len(row) < 4 {
			return "", nil, errors.New("resource group unknown")
		}
		return "az", []string{"aks", "get-credentials", "--name", name, "--resource-group", row[3], "--overwrite-existing"}, nil
	}
}

// cloudImport finds a managed Kubernetes cluster by name in the signed-in
// accounts and writes its credentials into the kubeconfig.
func (a *App) cloudImport(name string) {
	a.Flash().Infof("Looking for cluster %q in aws, gcp and azure…", name)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*cloudTimeout)
		defer cancel()
		type hit struct {
			provider string
			row      []string
		}
		var hits []hit
		for _, p := range []struct{ prov, svc string }{{"aws", "eks"}, {"gcp", "gke"}, {"azure", "aks"}} {
			svc, _ := findService(p.prov, p.svc)
			out, err := runCLI(ctx, cliOf(p.prov), svc.Args...)
			if err != nil {
				continue
			}
			rows, _ := parseCloud(svc, out)
			for _, r := range rows {
				if r[0] == name {
					hits = append(hits, hit{p.prov, r})
				}
			}
		}
		fail := func(format string, v ...any) {
			a.QueueUpdateDraw(func() { a.Flash().Errf(format, v...) })
		}
		switch len(hits) {
		case 0:
			fail("no cluster named %q found (are you signed in? try :cloud)", name)
			return
		case 1:
		default:
			fail("%q exists in more than one provider; rename or import it with the provider CLI", name)
			return
		}
		h := hits[0]
		region, project := "", ""
		if b, err := runCLI(ctx, "aws", "configure", "get", "region"); err == nil && h.provider == "aws" {
			region = strings.TrimSpace(string(b))
		}
		if b, err := runCLI(ctx, "gcloud", "config", "get-value", "project"); err == nil && h.provider == "gcp" {
			project = strings.TrimSpace(string(b))
		}
		cli, args, err := importPlan(h.provider, name, h.row, region, project)
		if err != nil {
			fail("%s", err)
			return
		}
		if _, err := runCLI(ctx, cli, args...); err != nil {
			fail("import failed: %s", err)
			return
		}
		a.QueueUpdateDraw(func() { a.Flash().Infof("Imported %s cluster %q. Pick it with :ctx", h.provider, name) })
	}()
}

// cliError picks the line of a CLI's stderr that says what went wrong: the first
// one mentioning an error, else the last non-empty line.
func cliError(stderr string, fallback error) string {
	var last string
	for _, l := range strings.Split(stderr, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		last = l
		if strings.Contains(strings.ToLower(l), "error") {
			return truncate(l, 200)
		}
	}
	if last == "" {
		return fallback.Error()
	}

	return truncate(last, 200)
}
