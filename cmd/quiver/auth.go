package main

import (
	"fmt"
	"net/url"
	"time"

	"github.com/spf13/cobra"
)

// apiResponse mirrors internal/api/libs.apiResponse. The CLI decodes into it
// directly rather than importing the api package, which would pull in Gin
// and the whole HTTP layer for a handful of JSON fields.
type apiResponse[T any] struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Data    T      `json:"data"`
}

type pairingCodeDTO struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

type deviceDTO struct {
	ID         string    `json:"id"`
	Label      string    `json:"label"`
	State      string    `json:"state"`
	PairedAt   time.Time `json:"paired_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

func newAuthCmd() *cobra.Command {
	var host string

	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage device pairing for the Quiver daemon",
	}
	cmd.PersistentFlags().StringVar(&host, "host", "",
		"daemon host URI (overrides config), e.g. unix:// or tcp://127.0.0.1:40257")

	cmd.AddCommand(newAuthGenerateCmd(&host))
	cmd.AddCommand(newAuthDevicesCmd(&host))

	return cmd
}

func newAuthGenerateCmd(host *string) *cobra.Command {
	return &cobra.Command{
		Use:   "generate",
		Short: "Generate a one-time pairing code for quiver.desktop",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := newDaemonClient(*host)
			if err != nil {
				return err
			}

			var resp apiResponse[pairingCodeDTO]
			if err := client.do(cmd.Context(), "POST", "/v0/auth/pairing", nil, &resp); err != nil {
				return err
			}
			if !resp.Success {
				return fmt.Errorf("generate pairing code: %s", resp.Error)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Pairing code: %s (expires %s)\n",
				resp.Data.Code, resp.Data.ExpiresAt.Format(time.RFC3339))
			return nil
		},
	}
}

func newAuthDevicesCmd(host *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devices",
		Short: "List or revoke paired devices",
	}
	cmd.AddCommand(newAuthDevicesListCmd(host))
	cmd.AddCommand(newAuthDevicesRevokeCmd(host))
	return cmd
}

func newAuthDevicesListCmd(host *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every device currently paired with this daemon",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := newDaemonClient(*host)
			if err != nil {
				return err
			}

			var resp apiResponse[[]deviceDTO]
			if err := client.do(cmd.Context(), "GET", "/v0/auth/devices", nil, &resp); err != nil {
				return err
			}
			if !resp.Success {
				return fmt.Errorf("list devices: %s", resp.Error)
			}

			printDevices(cmd, resp.Data)
			return nil
		},
	}
}

func printDevices(
	cmd *cobra.Command,
	devices []deviceDTO,
) {
	if len(devices) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No paired devices.")
		return
	}

	for _, d := range devices {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\tlast seen %s\n",
			d.ID, d.Label, d.State, d.LastSeenAt.Format(time.RFC3339))
	}
}

func newAuthDevicesRevokeCmd(host *string) *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <device-id>",
		Short: "Revoke a paired device's credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newDaemonClient(*host)
			if err != nil {
				return err
			}

			var resp apiResponse[struct{}]
			path := "/v0/auth/devices/" + url.PathEscape(args[0])
			if err := client.do(cmd.Context(), "DELETE", path, nil, &resp); err != nil {
				return err
			}
			if !resp.Success {
				return fmt.Errorf("revoke device: %s", resp.Error)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Revoked device %s\n", args[0])
			return nil
		},
	}
}
