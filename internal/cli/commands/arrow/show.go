package arrow

import (
	"strings"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func (c *commands) showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <namespace>",
		Short: "Show one catalog arrow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := clierr.ValidNS(args[0]); err != nil {
				return err
			}
			return invoke.RunInstant(
				c.sess, c.rb, cmd, "loading "+args[0],
				func(cli *client.Client) (apidto.ArrowDetailDTO, error) {
					return cli.GetArrow(cmd.Context(), args[0])
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

// ViewDetail renders one arrow's detail fields.
func ViewDetail(d apidto.ArrowDetailDTO, t theme.Theme) string {
	var fields []component.Field
	fields = field(fields, "Namespace", d.Namespace)
	fields = field(fields, "Name", d.Name)
	fields = field(fields, "State", t.State(domain.ArrowState(d.State)))
	fields = field(fields, "Description", d.Description)
	fields = field(fields, "License", d.License)
	fields = field(fields, "Tags", strings.Join(d.Tags, ", "))
	fields = field(fields, "Constraint", d.InstalledConstraint)
	fields = field(fields, "Installed", d.InstalledAt)
	return component.Fields("ARROW", fields, t)
}
