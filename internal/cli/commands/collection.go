package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/ui"
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
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			var collections []apidto.CollectionListItemDTO
			if err := a.withSpinner(cmd, "loading", func() error {
				var e error
				collections, e = cli.ListCollections(cmd.Context())
				return e
			}); err != nil {
				return err
			}
			return a.render(cmd, collections, func(w io.Writer) error {
				_, _ = fmt.Fprint(w, ui.CommandHeader("collection list", ""))
				_, _ = fmt.Fprintln(w)
				_, _ = fmt.Fprint(w, ui.RenderTable(
					[]string{"NAMESPACE", "NAME", "ARROWS"},
					collectionRows(collections),
				))
				return nil
			})
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
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			var detail apidto.CollectionDetailDTO
			if err := a.withSpinner(cmd, "loading", func() error {
				var e error
				detail, e = cli.GetCollection(cmd.Context(), args[0])
				return e
			}); err != nil {
				return err
			}
			return a.render(cmd, detail, func(w io.Writer) error {
				writeCollectionDetail(w, detail)
				return nil
			})
		},
	}
}

func writeCollectionDetail(w io.Writer, d apidto.CollectionDetailDTO) {
	_, _ = fmt.Fprint(w, ui.CommandHeader("collection", d.Namespace))
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "  %s — %s\n\n", ui.Bold.Render(d.Name), d.Description)

	rows := make([][]string, 0, len(d.Arrows))
	for _, arrow := range d.Arrows {
		resolved := "no"
		if arrow.Resolved {
			resolved = "yes"
		}
		rows = append(rows, []string{arrow.Namespace, arrow.Name, resolved})
	}
	_, _ = fmt.Fprint(w, ui.RenderTable([]string{"NAMESPACE", "NAME", "RESOLVED"}, rows))
}
