package auth

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

func (c *commands) devicesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every device currently paired with this daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return invoke.RunInstant(
				c.sess, c.rb, cmd, "loading devices",
				func(cli *client.Client) ([]client.Device, error) {
					return cli.ListDevices(cmd.Context())
				},
				viewDevices,
			)
		},
	}
}

func (c *commands) devicesRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <device-id>",
		Short: "Revoke a paired device's credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := c.sess.Client(cmd.Context(), cmd)
			if err != nil {
				return err
			}
			return cli.RevokeDevice(cmd.Context(), args[0])
		},
	}
}

func viewDevices(devices []client.Device, t theme.Theme) string {
	rows := make([][]string, 0, len(devices))
	for _, d := range devices {
		rows = append(rows, []string{d.ID, d.Label, d.State, d.LastSeenAt.Format("2006-01-02T15:04:05Z")})
	}
	return component.Table(
		[]component.Column{{Title: "ID"}, {Title: "LABEL"}, {Title: "STATE"}, {Title: "LAST SEEN"}},
		rows, "no paired devices", t,
	)
}
