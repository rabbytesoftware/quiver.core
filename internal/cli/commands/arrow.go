package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/ui"
)

func (a *app) arrowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "arrow",
		Short: "Manage the arrow catalog",
	}
	cmd.AddCommand(
		a.arrowAddCmd(), a.arrowRemoveCmd(), a.arrowRefreshCmd(),
		a.arrowListCmd(), a.arrowShowCmd(),
	)
	return cmd
}

func (a *app) arrowRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh <namespace>",
		Short: "Re-fetch an arrow's manifest from its source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validNS(args[0]); err != nil {
				return err
			}
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			if err := a.withSpinner(cmd, "refreshing "+args[0], func() error {
				return cli.RefreshArrow(cmd.Context(), args[0])
			}); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "refreshed %s\n", args[0])
			return nil
		},
	}
}

func (a *app) arrowAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <namespace>",
		Short: "Register an arrow in the catalog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validNS(args[0]); err != nil {
				return err
			}
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			if err := a.withSpinner(cmd, "adding "+args[0], func() error {
				return cli.AddArrow(cmd.Context(), args[0])
			}); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "added %s\n", args[0])
			return nil
		},
	}
}

func (a *app) arrowRemoveCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "remove <namespace>",
		Short: "Remove an arrow from the catalog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validNS(args[0]); err != nil {
				return err
			}
			if err := a.confirm(cmd, force, "remove "+args[0]); err != nil {
				return err
			}
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			if err := a.withSpinner(cmd, "removing "+args[0], func() error {
				return cli.RemoveArrow(cmd.Context(), args[0])
			}); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "y", false, "skip the confirmation prompt")
	return cmd
}

func (a *app) arrowListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List catalog arrows",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			arrows, err := cli.ListArrows(cmd.Context(), nil)
			if err != nil {
				return err
			}
			return a.render(cmd, arrows, func(w io.Writer) error {
				_, _ = fmt.Fprint(w, ui.CommandHeader("arrow list", ""))
				_, _ = fmt.Fprintln(w)
				_, _ = fmt.Fprint(w, ui.RenderTable(arrowTableHeaders(), arrowRows(arrows)))
				return nil
			})
		},
	}
}

func (a *app) arrowShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <namespace>",
		Short: "Show one catalog arrow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validNS(args[0]); err != nil {
				return err
			}
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			detail, err := cli.GetArrow(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return a.render(cmd, detail, func(w io.Writer) error {
				writeArrowDetail(w, detail)
				return nil
			})
		},
	}
}
