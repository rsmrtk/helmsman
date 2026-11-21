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
	Providers []string // aws | gcp | azure; empty means all three
	Regions   []string // AWS regions to scan; empty means the CLI default (aws configure / AWS_REGION)
	DryRun    bool     // list what would be imported, change nothing
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
		providers = []string{"aws", "gcp", "azure"}
	}
	prev := currentContext()
	imported, failed := 0, 0

	for _, p := range providers {
		prov, ok := providerOf(p)
		if !ok {
			return fmt.Errorf("unknown provider %q (use aws, gcp or azure)", p)
		}
		who, err := whoami(ctx, prov)
		if err != nil {
			fmt.Fprintf(w, "- %-5s skipped: %s\n", prov, err)

			continue
		}
		fmt.Fprintf(w, "- %-5s signed in as %s\n", prov, who)

		regions := []string{""}
		if prov == "aws" {
			if regions = awsRegions(ctx, o.Regions); len(regions) == 0 {
				fmt.Fprintln(w, "    no AWS region: pass --region <region> or run `aws configure set region <region>`")
				failed++

				continue
			}
		}
		for _, region := range regions {
			svc, _ := findService(prov, map[string]string{"aws": "eks", "gcp": "gke", "azure": "aks"}[prov])
			args := append([]string{}, svc.Args...)
			if prov == "aws" {
				args = append(args, "--region", region)
			}
			out, err := runCLI(ctx, cliOf(prov), args...)
			if err != nil {
				// A region the account may not use, or a service that is switched off (GKE API disabled, SCP-blocked
				// region): nothing to import there. That is a skip, not a failure, so discovery over several
				// providers does not end with an error exit code.
				fmt.Fprintf(w, "    %s: skipped, cannot list clusters: %s\n", firstNonEmpty(region, prov), err)

				continue
			}
			rows, _ := parseCloud(svc, out)
			if len(rows) == 0 {
				fmt.Fprintf(w, "    %s: no clusters\n", firstNonEmpty(region, prov))
			}
			for _, row := range rows {
				name := row[0]
				cli, cargs, err := importArgs(ctx, prov, name, row, region)
				if err != nil {
					fmt.Fprintf(w, "    x %s: %s\n", name, err)
					failed++

					continue
				}
				if o.DryRun {
					fmt.Fprintf(w, "    would import %s: %s %s\n", name, cli, strings.Join(cargs, " "))

					continue
				}
				if _, err := runCLI(ctx, cli, cargs...); err != nil {
					fmt.Fprintf(w, "    x %s: %s\n", name, err)
					failed++

					continue
				}
				fmt.Fprintf(w, "    + %s\n", name)
				imported++
			}
		}
	}
	if !o.DryRun {
		if err := restoreContext(prev); err != nil {
			fmt.Fprintf(w, "warning: could not restore your current context %q: %s\n", prev, err)
		}
	}
	switch {
	case o.DryRun:
		fmt.Fprintln(w, "dry run: nothing was changed")
	case imported > 0:
		fmt.Fprintf(w, "\nImported %d cluster(s). Your current context is unchanged (%s). Start `helmsman`.\n", imported, firstNonEmpty(prev, "none"))
	case failed == 0:
		fmt.Fprintln(w, "\nNo clusters found. Sign in first (aws login / gcloud auth login / az login) and run init again.")
	}
	if failed > 0 {
		return fmt.Errorf("%d cluster(s) could not be imported", failed)
	}

	return nil
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
	if provider == "gcp" {
		if b, err := runCLI(ctx, "gcloud", "config", "get-value", "project"); err == nil {
			project = strings.TrimSpace(string(b))
		}
	}
	cli, args, err = importPlan(provider, name, row, region, project)
	if err == nil && provider == "aws" {
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
