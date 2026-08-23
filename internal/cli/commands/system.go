package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *app) healthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check the daemon's health",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			if err := cli.Health(cmd.Context()); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "daemon: ok")
			return nil
		},
	}
}

func (a *app) versionCmd() *cobra.Command {
	var clientOnly bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show CLI and daemon versions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "client %s\n", a.deps.Version)
			if clientOnly {
				return nil
			}

			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			info, err := cli.Versions(cmd.Context())
			if err != nil {
				_, _ = fmt.Fprintf(w, "daemon unreachable: %v\n", err)
				return nil
			}
			_, _ = fmt.Fprintf(w, "daemon %s (build %s, api %s)\n",
				info.Version, info.BuildID, info.API.Latest)
			return nil
		},
	}
	cmd.Flags().BoolVar(&clientOnly, "client-only", false, "skip the daemon version lookup")
	return cmd
}
