package collection

import (
	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

func (c *commands) showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <namespace>",
		Short: "Show a collection and its arrows",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := clierr.ValidNS(args[0]); err != nil {
				return err
			}
			return invoke.RunInstant(
				c.sess, c.rb, cmd, "loading "+args[0],
				func(cli *client.Client) (apidto.CollectionDetailDTO, error) {
					return cli.GetCollection(cmd.Context(), args[0])
				},
				ViewDetail,
			)
		},
	}
}

// field appends a labelled value, dropping it when empty so a detail view
// shows only what the subject actually has.
func field(fields []component.Field, label, value string) []component.Field {
	if value == "" {
		return fields
	}
	return append(fields, component.Field{Label: label, Value: value, Set: true})
}

// ViewDetail renders one collection's detail fields and its arrow membership
// table.
func ViewDetail(d apidto.CollectionDetailDTO, t theme.Theme) string {
	var fields []component.Field
	fields = field(fields, "Namespace", d.Namespace)
	fields = field(fields, "Name", d.Name)
	fields = field(fields, "Description", d.Description)

	rows := make([][]string, 0, len(d.Arrows))
	for _, arrow := range d.Arrows {
		resolved := "no"
		if arrow.Resolved {
			resolved = "yes"
		}
		rows = append(rows, []string{arrow.Namespace, arrow.Name, resolved})
	}

	return component.Fields("COLLECTION", fields, t) + "\n" +
		t.Header.Render("ARROWS") + "\n" +
		component.Table(
			[]component.Column{
				{Title: "NAMESPACE"}, {Title: "NAME"}, {Title: "RESOLVED"},
			},
			rows, "no arrows", t,
		)
}
