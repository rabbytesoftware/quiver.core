package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/rabbytesoftware/quiver/internal"
)

func newDaemonCmd() *cobra.Command {
	var host string
	var port int

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the Quiver API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			container, err := internal.Init(ctx)
			if err != nil {
				return err
			}

			slog.Info("starting quiver daemon")
			// TODO(Task 5): Update to Run(host, port) when signature changes
			addr := fmt.Sprintf("%s:%d", host, port)
			return container.API.Run(addr)
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "host to bind (overrides config)")
	cmd.Flags().IntVar(&port, "port", 0, "port to listen on (overrides config)")

	return cmd
}
