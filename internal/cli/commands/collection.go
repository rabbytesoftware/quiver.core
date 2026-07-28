package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
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

// collectionAction wraps the shared validate → session → call → confirm shape.
func (a *app) collectionAction(
	use, short, done string,
	call func(cmd *cobra.Command, ns string) error,
) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <namespace>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validNS(args[0]); err != nil {
				return err
			}
			if err := call(cmd, args[0]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", done, args[0])
			return nil
		},
	}
}

func (a *app) collectionFollowCmd() *cobra.Command {
	return a.collectionAction(
		"follow", "Follow a collection", "following",
		func(cmd *cobra.Command, ns string) error {
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			return a.withSpinner(cmd, "following "+ns, func() error {
				return cli.FollowCollection(cmd.Context(), ns)
			})
		},
	)
}

func (a *app) collectionUnfollowCmd() *cobra.Command {
	var force bool
	cmd := a.collectionAction(
		"unfollow", "Unfollow a collection", "unfollowed",
		func(cmd *cobra.Command, ns string) error {
			if err := a.confirm(cmd, force, "unfollow "+ns); err != nil {
				return err
			}
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			return a.withSpinner(cmd, "unfollowing "+ns, func() error {
				return cli.UnfollowCollection(cmd.Context(), ns)
			})
		},
	)
	cmd.Flags().BoolVar(&force, "force", false, "skip the confirmation prompt")
	return cmd
}

func (a *app) collectionUpdateCmd() *cobra.Command {
	return a.collectionAction(
		"update", "Re-resolve a collection from its source", "updated",
		func(cmd *cobra.Command, ns string) error {
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			return a.withSpinner(cmd, "updating "+ns, func() error {
				return cli.UpdateCollection(cmd.Context(), ns)
			})
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
			collections, err := cli.ListCollections(cmd.Context())
			if err != nil {
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
			detail, err := cli.GetCollection(cmd.Context(), args[0])
			if err != nil {
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
		rows = append(rows, []string{arrow.Namespace, arrow.Name, arrow.Version})
	}
	_, _ = fmt.Fprint(w, ui.RenderTable([]string{"NAMESPACE", "NAME", "VERSION"}, rows))
}
