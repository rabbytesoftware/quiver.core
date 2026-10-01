package arrow

import (
	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func (c *commands) listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List catalog arrows",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return invoke.RunInstant(
				c.sess, c.rb, cmd, "loading arrows",
				func(cli *client.Client) ([]output.ArrowRow, error) {
					arrows, err := cli.ListArrows(cmd.Context(), nil)
					if err != nil {
						return nil, err
					}
					return RowsFrom(arrows), nil
				},
				Table,
			)
		},
	}
}

func columns() []component.Column {
	return []component.Column{
		{Title: "NAMESPACE"}, {Title: "NAME"}, {Title: "REF"}, {Title: "RESOLVED"}, {Title: "STATE"},
	}
}

// Table renders arrow rows. Exported so commands/discovery can share it
// for the top-level `quiver list`/`quiver search`.
func Table(arrows []output.ArrowRow, t theme.Theme) string {
	return component.Table(columns(), rows(arrows, t), "no arrows", t)
}

func rows(arrows []output.ArrowRow, t theme.Theme) [][]string {
	out := make([][]string, 0, len(arrows))
	for _, arrow := range arrows {
		out = append(out, []string{
			arrow.Namespace, arrow.Name, arrow.Ref, arrow.Resolved, t.State(domain.ArrowState(arrow.State)),
		})
	}
	return out
}

// RowsFrom shapes the API listing into the row type Table renders.
// Exported for the same reason as Table.
func RowsFrom(arrows []apidto.ArrowListItemDTO) []output.ArrowRow {
	out := make([]output.ArrowRow, 0, len(arrows))
	for _, arrow := range arrows {
		out = append(out, RowFrom(arrow))
	}
	return out
}

// RowFrom reads the arrow's first version into a row. An arrow with no
// versions is in the catalog but not installed anywhere. Exported for the
// same reason as Table and RowsFrom.
func RowFrom(arrow apidto.ArrowListItemDTO) output.ArrowRow {
	row := output.ArrowRow{Namespace: arrow.Namespace, Name: arrow.Name, Ref: "-", Resolved: "-", State: "absent"}
	if len(arrow.Versions) == 0 {
		return row
	}
	v := arrow.Versions[0]
	row.Ref, row.Resolved, row.State = orDash(v.Ref), orDash(v.ResolvedRef), v.State
	return row
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
