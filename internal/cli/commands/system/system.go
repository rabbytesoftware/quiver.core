// Package system implements `quiver health` and `quiver version`.
package system

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
)

type Commands interface {
	Cmd() []*cobra.Command
}

type commands struct {
	sess    session.Session
	version string
}

// New returns a Commands reporting version for `quiver version --client-only`.
func New(
	sess session.Session,
	version string,
) Commands {
	return &commands{sess: sess, version: version}
}

// Cmd returns health and version as two root-level commands — neither
// nests under a shared "system" subcommand today, so this returns a slice
// rather than one parent Cmd() like every other resource package.
func (c *commands) Cmd() []*cobra.Command {
	return []*cobra.Command{c.healthCmd(), c.versionCmd()}
}

func (c *commands) healthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check the daemon's health",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := c.sess.Client(cmd.Context(), cmd)
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

func (c *commands) versionCmd() *cobra.Command {
	var clientOnly bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show CLI and daemon versions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "client %s\n", c.version)
			if clientOnly {
				return nil
			}

			cli, err := c.sess.Client(cmd.Context(), cmd)
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
