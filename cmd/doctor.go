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

func doctorCmd() *cobra.Command {
	var o view.DoctorOptions

	command := cobra.Command{
		Use:   "doctor",
		Short: "Check what helmsman needs: kubeconfig, cluster, terminal, Nerd Font, cloud CLIs",
		Long: `Looks at everything helmsman depends on and says what to fix: the kubeconfig and the cluster behind the current
context, your terminal (colors, animation, the big heading glyph), a Nerd Font for the icons, and the cloud CLIs
that "helmsman init" uses. It changes nothing; a failed check makes it exit with a non-zero code.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			o.Version = version

			return view.Doctor(ctx, cmd.OutOrStdout(), o)
		},
	}
	command.Flags().BoolVar(&o.NoCloud, "no-cloud", false, "skip the cloud CLI sign-in checks")

	return &command
}
