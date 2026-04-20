package main

import (
	"context"
	"log/slog"

	"github.com/rabbytesoftware/quiver/internal"
	"github.com/spf13/cobra"
)

func newDaemonCmd() *cobra.Command {
	var host string
	var port int

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the Quiver API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			container, err := internal.New(ctx)
			if err != nil {
				return err
			}

			slog.Info("starting quiver daemon", "version", version, "build", buildID)
			return container.API.Run(host, port)
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "host to bind (overrides config)")
	cmd.Flags().IntVar(&port, "port", 0, "port to listen on (overrides config)")

	return cmd
}
