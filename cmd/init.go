// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package cmd

import (
	"context"
	"os"
	"os/signal"
	"time"

	"github.com/rsmrtk/helmsman/internal/view"
	"github.com/spf13/cobra"
)

const initLong = `Finds the managed Kubernetes clusters in the accounts you are already signed in to (aws, gcloud, az) and writes
their credentials into your kubeconfig, so they appear in helmsman, kubectl and k9s from any terminal without
exporting KUBECONFIG. Your current context is left as it was. helmsman stores no credentials.

On a terminal each provider gets its own animated banner while the cloud CLI works. Set HELMSMAN_NO_ANIM=1 or pass
--no-animation for plain text; HELMSMAN_ICONS=plain replaces the Nerd Font icons with text.`

func initCmd() *cobra.Command {
	var o view.InitOptions

	run := func(providers ...string) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			o.Providers = providers

			return view.InitClusters(ctx, cmd.OutOrStdout(), o)
		}
	}

	command := cobra.Command{
		Use:   "init",
		Short: "Import your cloud clusters (EKS, GKE, AKS) into the kubeconfig",
		Long:  initLong,
		Example: `  helmsman init                       # every provider you are signed in to
  helmsman init aws --region eu-north-1
  helmsman init gcp
  helmsman init azure --dry-run       # show what would be imported`,
		SilenceUsage: true,
		RunE:         run(),
	}
	command.PersistentFlags().BoolVar(&o.DryRun, "dry-run", false, "list what would be imported, change nothing")
	command.PersistentFlags().BoolVar(&o.NoAnim, "no-animation", false, "plain text, no animated banner")
	command.PersistentFlags().StringSliceVar(&o.Regions, "region", nil, "AWS region to scan (repeatable; AWS only; default: your AWS CLI region)")

	for _, p := range []struct{ use, short, prov string }{
		{"aws", "Import Amazon EKS clusters", "aws"},
		{"gcp", "Import Google Kubernetes Engine (GKE) clusters", "gcp"},
		{"azure", "Import Azure Kubernetes Service (AKS) clusters", "azure"},
	} {
		command.AddCommand(&cobra.Command{
			Use:          p.use,
			Short:        p.short,
			Long:         initLong,
			SilenceUsage: true,
			Args:         cobra.NoArgs,
			RunE:         run(p.prov),
		})
	}

	command.AddCommand(&cobra.Command{
		Use:          "demo",
		Short:        "Play the loading animation of every provider (no cloud is contacted)",
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
