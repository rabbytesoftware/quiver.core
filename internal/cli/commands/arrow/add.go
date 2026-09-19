package arrow

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
)

func (c *commands) addCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <namespace>",
		Short: "Register an arrow in the catalog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := clierr.ValidNS(args[0]); err != nil {
				return err
			}
			return invoke.RunMutation(c.sess, c.rb, cmd, output.ActionAdd, args[0],
				func(cli *client.Client) error {
					return cli.AddArrow(cmd.Context(), args[0])
				})
		},
	}
}
