package main

import (
	"context"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal"
)

func newDaemonCmd() *cobra.Command {
	var host string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the Quiver API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			container, err := internal.New(ctx, version, buildID)
			if err != nil {
				return err
			}

			slog.Info("starting quiver daemon", "version", version, "build", buildID)
			return container.Start(ctx, host)
		},
	}

	cmd.Flags().StringVar(&host, "host", "", `host URI to bind (overrides config). Examples:
  unix:///custom/path/quiver.sock   Unix domain socket at custom path
  unix://                           Unix domain socket at default path (~/.quiver/quiver.sock)
  tcp://0.0.0.0:40257               TCP socket (remote mode)`)

	return cmd
}
