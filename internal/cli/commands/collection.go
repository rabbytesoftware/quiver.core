package commands

import (
	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
)

func (a *app) collectionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collection",
		Short: "Manage followed collections",
	}
	cmd.AddCommand(
		a.collectionFollowCmd(), a.collectionUnfollowCmd(),
		a.collectionListCmd(), a.collectionShowCmd(), a.collectionUpdateCmd(),
	)
	return cmd
}

// collectionAction builds one collection mutation. before runs after argument
// validation and before the daemon is contacted, which is where the
// confirmation gate belongs: a cancelled command must not boot a daemon.
func (a *app) collectionAction(
	use, short string,
	action output.Action,
	before func(cmd *cobra.Command, ns string) error,
	call func(cli *client.Client, cmd *cobra.Command, ns string) error,
) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <namespace>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validNS(args[0]); err != nil {
				return err
			}
			if before != nil {
				if err := before(cmd, args[0]); err != nil {
					return err
				}
			}
			return a.runMutation(cmd, action, args[0], func(cli *client.Client) error {
				return call(cli, cmd, args[0])
			})
		},
	}
}

func (a *app) collectionFollowCmd() *cobra.Command {
	return a.collectionAction(
		"follow", "Follow a collection", output.ActionFollow, nil,
		func(cli *client.Client, cmd *cobra.Command, ns string) error {
			return cli.FollowCollection(cmd.Context(), ns)
		},
	)
}

func (a *app) collectionUnfollowCmd() *cobra.Command {
	var yes bool
	cmd := a.collectionAction(
		"unfollow", "Unfollow a collection", output.ActionUnfollow,
		func(cmd *cobra.Command, ns string) error {
			return a.confirm(cmd, yes, "unfollow "+ns)
		},
		func(cli *client.Client, cmd *cobra.Command, ns string) error {
			return cli.UnfollowCollection(cmd.Context(), ns)
		},
	)
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func (a *app) collectionUpdateCmd() *cobra.Command {
	return a.collectionAction(
		"update", "Re-resolve a collection from its source", output.ActionUpdate, nil,
		func(cli *client.Client, cmd *cobra.Command, ns string) error {
			return cli.UpdateCollection(cmd.Context(), ns)
		},
	)
}

func (a *app) collectionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List followed collections",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInstant(
				a, cmd, "loading collections",
				func(cli *client.Client) ([]output.CollectionRow, error) {
					collections, err := cli.ListCollections(cmd.Context())
					if err != nil {
						return nil, err
					}

					return collectionRowsFrom(collections), nil
				},
				collectionTable,
			)
		},
	}
}

func (a *app) collectionShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <namespace>",
		Short: "Show a collection and its arrows",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validNS(args[0]); err != nil {
				return err
			}

			return runInstant(
				a, cmd, "loading "+args[0],
				func(cli *client.Client) (apidto.CollectionDetailDTO, error) {
					return cli.GetCollection(cmd.Context(), args[0])
				},
				viewCollectionDetail,
			)
		},
	}
}
