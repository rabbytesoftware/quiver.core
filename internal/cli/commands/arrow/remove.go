package arrow

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
)

func (c *commands) removeCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <namespace>",
		Short: "Remove an arrow from the catalog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := clierr.ValidNS(args[0]); err != nil {
				return err
			}
			if err := clierr.Confirm(cmd, c.sess.IsTTY(), yes, "remove "+args[0]); err != nil {
				return err
			}
			return invoke.RunMutation(c.sess, c.rb, cmd, output.ActionRemove, args[0],
				func(cli *client.Client) error {
					return cli.RemoveArrow(cmd.Context(), args[0])
				})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}
