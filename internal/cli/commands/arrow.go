package commands

import (
	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
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
			return a.runMutation(cmd, output.ActionRefresh, args[0], func(cli *client.Client) error {
				return cli.RefreshArrow(cmd.Context(), args[0])
			})
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
			return a.runMutation(cmd, output.ActionAdd, args[0], func(cli *client.Client) error {
				return cli.AddArrow(cmd.Context(), args[0])
			})
		},
	}
}

func (a *app) arrowRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <namespace>",
		Short: "Remove an arrow from the catalog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validNS(args[0]); err != nil {
				return err
			}
			if err := a.confirm(cmd, yes, "remove "+args[0]); err != nil {
				return err
			}
			return a.runMutation(cmd, output.ActionRemove, args[0], func(cli *client.Client) error {
				return cli.RemoveArrow(cmd.Context(), args[0])
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func (a *app) arrowListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List catalog arrows",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInstant(
				a, cmd, "loading arrows",
				func(cli *client.Client) ([]output.ArrowRow, error) {
					arrows, err := cli.ListArrows(cmd.Context(), nil)
					if err != nil {
						return nil, err
					}

					return arrowRowsFrom(arrows), nil
				},
				arrowTable,
			)
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

			return runInstant(
				a, cmd, "loading "+args[0],
				func(cli *client.Client) (apidto.ArrowDetailDTO, error) {
					return cli.GetArrow(cmd.Context(), args[0])
				},
				viewArrowDetail,
			)
		},
	}
}
