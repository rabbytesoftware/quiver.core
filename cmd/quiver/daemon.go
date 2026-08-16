package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal"
	"github.com/rabbytesoftware/quiver.core/internal/core/build"
)

func newDaemonCmd() *cobra.Command {
	var host string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the Quiver API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			buildInfo, err := resolveBuildInfo(ctx)
			if err != nil {
				return err
			}

			container, err := internal.New(ctx, buildInfo)
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

const updateAttemptTokenEnv = "QUIVER_UPDATE_ATTEMPT_TOKEN"

func resolveBuildInfo(ctx context.Context) (build.Info, error) {
	return resolveBuildInfoWith(ctx, os.Getenv(updateAttemptTokenEnv), os.Executable, build.Digest)
}

func resolveBuildInfoWith(
	ctx context.Context,
	attemptToken string,
	executable func() (string, error),
	digest func(context.Context, string) (string, error),
) (build.Info, error) {
	path, err := executable()
	if err != nil {
		return build.Info{}, fmt.Errorf("daemon: resolve executable: %w", err)
	}
	sha256, err := digest(ctx, path)
	if err != nil {
		return build.Info{}, fmt.Errorf("daemon: %w", err)
	}
	return build.Info{
		Version:      version,
		BuildID:      buildID,
		Digest:       sha256,
		AttemptToken: attemptToken,
	}, nil
}
