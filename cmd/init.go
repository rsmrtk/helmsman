// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/rsmrtk/helmsman/internal/view"
	"github.com/spf13/cobra"
)

const initLong = `Makes your clusters visible to helmsman, kubectl and k9s from any terminal, with no "export KUBECONFIG": it writes
them into your kubeconfig. Your current context is left as it was, and helmsman stores no credentials.

Targets (any number, in any order):
  all      everything below (this is also what plain "helmsman init" does)
  local    clusters on this machine or in the project you are in: kubeconfig files in the current git repository
           (build/kubeconfig-eks, kubeconfig.yaml, .kube/config ...), and kind, k3d and minikube clusters
  aws      Amazon EKS clusters, through the aws CLI you are signed in to
  gcp      Google GKE clusters, through gcloud
  azure    Azure AKS clusters, through az

On a terminal each target gets its own animated logo while it works. Set HELMSMAN_NO_ANIM=1 or pass --no-animation for
plain text; HELMSMAN_ICONS=plain replaces the Nerd Font icons with text.`

// initTargets are the words "helmsman init" understands, besides the subcommand "demo".
var initTargets = []string{"all", tLocal, tAWS, tGCP, tAzure}

// the names of the targets
const (
	tLocal = "local"
	tAWS   = "aws"
	tGCP   = "gcp"
	tAzure = "azure"
)

// everyTarget is the order targets always run in.
var everyTarget = []string{tLocal, tAWS, tGCP, tAzure}

// parseInitTargets turns the arguments into the ordered, de-duplicated list of targets to run. "all" and no arguments
// mean every target; the order is always local, aws, gcp, azure whatever order the user typed.
func parseInitTargets(args []string) ([]string, error) {
	if len(args) == 0 {
		return everyTarget, nil
	}
	want := map[string]bool{}
	for _, a := range args {
		a = strings.ToLower(a)
		switch a {
		case "all":
			return everyTarget, nil
		case tLocal, tAWS, tGCP, tAzure:
			want[a] = true
		default:
			return nil, fmt.Errorf("unknown target %q: use %s", a, strings.Join(initTargets, ", "))
		}
	}
	var out []string
	for _, name := range everyTarget {
		if want[name] {
			out = append(out, name)
		}
	}

	return out, nil
}

func initCmd() *cobra.Command {
	var o view.InitOptions

	command := cobra.Command{
		Use:   "init [all | local | aws | gcp | azure]...",
		Short: "Import your clusters (local, EKS, GKE, AKS) into the kubeconfig",
		Long:  initLong,
		Example: `  helmsman init                          # same as: helmsman init all
  helmsman init all                      # local clusters and every cloud you are signed in to
  helmsman init local                    # only this machine and this project
  helmsman init local aws                # local clusters and Amazon EKS
  helmsman init aws --region eu-north-1  # only EKS, in that region
  helmsman init all --dry-run            # show what would be imported
  helmsman init demo                     # play the animations, contact nothing`,
		SilenceUsage: true,
		ValidArgs:    initTargets,
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return initTargets, cobra.ShellCompDirectiveNoFileComp
		},
		Args: func(_ *cobra.Command, args []string) error {
			_, err := parseInitTargets(args)

			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			targets, err := parseInitTargets(args)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			o.Providers = targets

			return view.InitClusters(ctx, cmd.OutOrStdout(), o)
		},
	}
	command.PersistentFlags().BoolVar(&o.DryRun, "dry-run", false, "list what would be imported, change nothing")
	command.PersistentFlags().BoolVar(&o.NoAnim, "no-animation", false, "plain text, no animated logo")
	command.PersistentFlags().StringSliceVar(&o.Regions, "region", nil, "AWS region to scan (repeatable; AWS only; default: your AWS CLI region)")

	command.AddCommand(&cobra.Command{
		Use:          "demo",
		Short:        "Play the loading animation of every target (nothing is contacted)",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			return view.InitDemo(ctx, cmd.OutOrStdout(), 5*time.Second)
		},
	})

	return &command
}
