package arrow

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
)

func (c *commands) refreshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh <namespace>",
		Short: "Re-fetch an arrow's manifest from its source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := clierr.ValidNS(args[0]); err != nil {
				return err
			}
			return invoke.RunMutation(c.sess, c.rb, cmd, output.ActionRefresh, args[0],
				func(cli *client.Client) error {
					return cli.RefreshArrow(cmd.Context(), args[0])
				})
		},
	}
}
