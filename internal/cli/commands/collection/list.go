package collection

import (
	"strconv"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

func (c *commands) listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List followed collections",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return invoke.RunInstant(
				c.sess, c.rb, cmd, "loading collections",
				func(cli *client.Client) ([]output.CollectionRow, error) {
					collections, err := cli.ListCollections(cmd.Context())
					if err != nil {
						return nil, err
					}
					return RowsFrom(collections), nil
				},
				Table,
			)
		},
	}
}

func columns() []component.Column {
	return []component.Column{
		{Title: "NAMESPACE"}, {Title: "NAME"}, {Title: "ARROWS"},
	}
}

// Table renders collection rows. Exported so commands/discovery can share it
// for the top-level `quiver list`/`quiver search`.
func Table(collections []output.CollectionRow, t theme.Theme) string {
	return component.Table(columns(), rows(collections), "no collections", t)
}

func rows(collections []output.CollectionRow) [][]string {
	out := make([][]string, 0, len(collections))
	for _, col := range collections {
		out = append(out, []string{col.Namespace, col.Name, strconv.Itoa(col.Arrows)})
	}
	return out
}

// RowsFrom shapes the API listing into the row type Table renders.
// Exported for the same reason as Table.
func RowsFrom(collections []apidto.CollectionListItemDTO) []output.CollectionRow {
	out := make([]output.CollectionRow, 0, len(collections))
	for _, col := range collections {
		out = append(out, output.CollectionRow{
			Namespace: col.Namespace, Name: col.Name, Arrows: col.ArrowCount,
		})
	}
	return out
}
