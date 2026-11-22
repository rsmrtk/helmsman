// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
)

// InitOptions controls `helmsman init`.
type InitOptions struct {
	Providers []string // local | aws | gcp | azure; empty means all four
	Regions   []string // AWS regions to scan; empty means the CLI default (aws configure / AWS_REGION)
	DryRun    bool     // list what would be imported, change nothing
	NoAnim    bool     // plain output even on a terminal
}

// InitClusters finds the managed Kubernetes clusters (EKS, GKE, AKS) in the accounts the provider CLIs are already
// signed in to and writes their credentials into the default kubeconfig. After it the clusters show up in helmsman,
// kubectl and k9s from any terminal, with no `export KUBECONFIG=...`.
//
// It never changes which context is current: importing a cluster must not silently redirect the next `kubectl` command
// at it. helmsman stores no credentials; everything goes through aws / gcloud / az.
func InitClusters(ctx context.Context, w io.Writer, o InitOptions) error {
	providers := o.Providers
	if len(providers) == 0 {
		providers = allTargets
	}
	for _, p := range providers {
		if _, ok := providerOf(p); !ok && p != provLocal {
			return fmt.Errorf("unknown target %q (use all, local, aws, gcp or azure)", p)
		}
	}
	ui := newInitUI(w, o.NoAnim)
	ui.begin()
	defer ui.end()
	ui.header(o.DryRun)
	prev := currentContext()
	imported, problems := 0, 0

	for _, p := range providers {
		prov := p
		if p != provLocal {
			prov, _ = providerOf(p)
		}
		var imp, prob int
		ui.stage(prov, func() {
			if prov == provLocal {
				imp, prob = initLocal(ctx, ui, o)

				return
			}
			imp, prob = initProvider(ctx, ui, prov, o)
		})
		imported += imp
		problems += prob
	}
	if !o.DryRun {
		if err := restoreContext(prev); err != nil {
			ui.fail("context", "could not restore your current context "+prev+": "+err.Error())
		}
	}
	ui.summary(imported, problems, o.DryRun, prev)

	return nil
}

// initProvider does the work for one provider and reports what it found through ui. It returns how many clusters it
// imported and how many problems it hit.
func initProvider(ctx context.Context, ui *initUI, prov string, o InitOptions) (imported, problems int) {
	defer ui.blank()
	who, err := step(ui, "checking your sign-in…", func() (string, error) { return whoami(ctx, prov) })
	if err != nil {
		reason, hint := explain(prov, err.Error(), "")
		ui.skip("account", reason)
		ui.hint(hint)

		return 0, 0
	}
	identity, scope := identityOf(prov, who)
	ui.ok("account", identity+scopeSuffix(scope))
	project := ""
	if prov == provGCP {
		if m := reProject.FindStringSubmatch(scope); m != nil {
			project = m[1]
		}
	}

	regions := []string{""}
	if prov == provAWS {
		if regions = awsRegions(ctx, o.Regions); len(regions) == 0 {
			ui.fail("region", "no AWS region configured")
			ui.hint("set a default region, or pass --region: run: aws configure set region <region>")

			return 0, 1
		}
	}
	svcName := clusterService[prov]
	for _, region := range regions {
		label := strings.ToUpper(svcName)
		if region != "" {
			label = region
		}
		svc, _ := findService(prov, svcName)
		args := append([]string{}, svc.Args...)
		if prov == provAWS {
			args = append(args, "--region", region)
		}
		out, err := step(ui, "listing "+strings.ToUpper(svcName)+" clusters"+inRegion(region)+"…", func() ([]byte, error) {
			return runCLI(ctx, cliOf(prov), args...)
		})
		if err != nil {
			reason, hint := explain(prov, err.Error(), project)
			ui.fail(label, reason)
			ui.hint(hint)
			problems++

			continue
		}
		rows, _ := parseCloud(svc, out)
		if len(rows) == 0 {
			ui.skip(label, "no clusters")
		}
		for _, row := range rows {
			name := row[0]
			cli, cargs, err := importArgs(ctx, prov, name, row, region)
			if err != nil {
				reason, hint := explain(prov, err.Error(), project)
				ui.fail(name, reason)
				ui.hint(hint)
				problems++

				continue
			}
			if o.DryRun {
				ui.would(name, cli+" "+strings.Join(cargs, " "))

				continue
			}
			if _, err := step(ui, "importing "+name+"…", func() ([]byte, error) { return runCLI(ctx, cli, cargs...) }); err != nil {
				reason, hint := explain(prov, err.Error(), project)
				ui.fail(name, reason)
				ui.hint(hint)
				problems++

				continue
			}
			ui.added(name)
			imported++
		}
	}

	return imported, problems
}

// clusterService is the managed Kubernetes service of each provider (the cloud service table in cloud_data.go keys on it).
var clusterService = map[string]string{provAWS: svcEKS, provGCP: svcGKE, provAzure: svcAKS}

const (
	svcEKS = "eks"
	svcGKE = "gke"
	svcAKS = "aks"
)

// allTargets is what `helmsman init all` (and a plain `helmsman init`) covers, in this order.
var allTargets = []string{provLocal, provAWS, provGCP, provAzure}

var providerTitle = map[string]string{provLocal: "Kubernetes", provAWS: "AWS", provGCP: "Google Cloud", provAzure: "Azure"}

func inRegion(region string) string {
	if region == "" {
		return ""
	}

	return " in " + region
}

func scopeSuffix(scope string) string {
	if scope == "" {
		return ""
	}

	return "  ·  " + scope
}

// awsRegions is the list of regions to scan: the flags, else the CLI default, else AWS_REGION / AWS_DEFAULT_REGION.
func awsRegions(ctx context.Context, flags []string) []string {
	if len(flags) > 0 {
		return flags
	}
	if b, err := runCLI(ctx, "aws", "configure", "get", "region"); err == nil {
		if r := strings.TrimSpace(string(b)); r != "" {
			return []string{r}
		}
	}
	for _, env := range []string{"AWS_REGION", "AWS_DEFAULT_REGION"} {
		if r := strings.TrimSpace(os.Getenv(env)); r != "" {
			return []string{r}
		}
	}

	return nil
}

// importArgs is importPlan plus two details that only matter for a bulk import: the project for GKE, and a context name
// equal to the cluster name for EKS (the default is the full ARN, which is unreadable in a context list).
func importArgs(ctx context.Context, provider, name string, row []string, region string) (cli string, args []string, err error) {
	project := ""
	if provider == provGCP {
		if b, perr := runCLI(ctx, "gcloud", "config", "get-value", "project"); perr == nil {
			project = strings.TrimSpace(string(b))
		}
	}
	cli, args, err = importPlan(provider, name, row, region, project)
	if err == nil && provider == provAWS {
		args = append(args, "--alias", name)
	}

	return cli, args, err
}

func currentContext() string {
	cfg, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return ""
	}

	return cfg.CurrentContext
}

// restoreContext puts the current-context back to prev when an import moved it. A context that no longer exists is left alone.
func restoreContext(prev string) error {
	if prev == "" {
		return nil
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg, err := rules.Load()
	if err != nil {
		return err
	}
	if cfg.CurrentContext == prev {
		return nil
	}
	if _, ok := cfg.Contexts[prev]; !ok {
		return errors.New("it no longer exists in the kubeconfig")
	}
	cfg.CurrentContext = prev

	return clientcmd.ModifyConfig(rules, *cfg, true)
}
