// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/rsmrtk/helmsman/internal/view"
	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	var o view.InitOptions

	command := cobra.Command{
		Use:   "init",
		Short: "Import your cloud clusters (EKS, GKE, AKS) into the kubeconfig",
		Long: `Finds the managed Kubernetes clusters in the accounts you are already signed in to (aws, gcloud, az) and writes
their credentials into your kubeconfig, so they appear in helmsman, kubectl and k9s from any terminal without
exporting KUBECONFIG. Your current context is left as it was. helmsman stores no credentials.`,
		Example: `  helmsman init                       # every provider you are signed in to
  helmsman init --provider aws --region eu-north-1
  helmsman init --dry-run             # show what would be imported`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			return view.InitClusters(ctx, cmd.OutOrStdout(), o)
		},
	}
	command.Flags().StringSliceVar(&o.Providers, "provider", nil, "only this provider: aws, gcp or azure (repeatable)")
	command.Flags().StringSliceVar(&o.Regions, "region", nil, "AWS region to scan (repeatable; default: your AWS CLI region)")
	command.Flags().BoolVar(&o.DryRun, "dry-run", false, "list what would be imported, change nothing")

	return &command
}
