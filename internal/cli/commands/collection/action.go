package collection

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
)

// action builds one collection mutation. before runs after argument
// validation and before the daemon is contacted, which is where the
// confirmation gate belongs: a cancelled command must not boot a daemon.
func (c *commands) action(
	use, short string,
	act output.Action,
	before func(cmd *cobra.Command, ns string) error,
	call func(cli *client.Client, cmd *cobra.Command, ns string) error,
) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <namespace>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := clierr.ValidNS(args[0]); err != nil {
				return err
			}
			if before != nil {
				if err := before(cmd, args[0]); err != nil {
					return err
				}
			}
			return invoke.RunMutation(c.sess, c.rb, cmd, act, args[0],
				func(cli *client.Client) error { return call(cli, cmd, args[0]) })
		},
	}
}

func (c *commands) followCmd() *cobra.Command {
	return c.action(
		"follow", "Follow a collection", output.ActionFollow, nil,
		func(cli *client.Client, cmd *cobra.Command, ns string) error {
			return cli.FollowCollection(cmd.Context(), ns)
		},
	)
}

func (c *commands) unfollowCmd() *cobra.Command {
	var yes bool
	cmd := c.action(
		"unfollow", "Unfollow a collection", output.ActionUnfollow,
		func(cmd *cobra.Command, ns string) error {
			return clierr.Confirm(cmd, c.sess.IsTTY(), yes, "unfollow "+ns)
		},
		func(cli *client.Client, cmd *cobra.Command, ns string) error {
			return cli.UnfollowCollection(cmd.Context(), ns)
		},
	)
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func (c *commands) updateCmd() *cobra.Command {
	return c.action(
		"update", "Re-resolve a collection from its source", output.ActionUpdate, nil,
		func(cli *client.Client, cmd *cobra.Command, ns string) error {
			return cli.UpdateCollection(cmd.Context(), ns)
		},
	)
}
