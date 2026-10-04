package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
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
			if err := scopeDevHome(version); err != nil {
				return err
			}

			// Before anything can write a log line: whoever is reading this
			// daemon's stdout may exit at any moment, and on unix that would
			// otherwise kill it. See surviveBrokenLogPipe.
			defer surviveBrokenLogPipe()()

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			// Bound before internal.New, not after: see PrepareGateway's own
			// doc for why a daemon that loses this race must be stopped here,
			// before New's construction ever reaches the sqlite migration.
			listener, scheme, err := internal.PrepareGateway(host)
			if err != nil {
				return err
			}

			container, err := internal.New(
				ctx, version, buildID,
				internal.WithCommit(commit),
				internal.WithChannel(channel),
				internal.WithStop(stop),
				internal.WithGateway(listener, scheme),
				internal.WithRecommendations(),
			)
			if err != nil {
				_ = listener.Close()
				return err
			}

			slog.Info("starting quiver daemon", "version", version, "build", buildID)

			return container.Start(ctx, host)
		},
	}

	cmd.Flags().StringVar(&host, "host", "", `host URI to bind (overrides config). Examples:
  unix:///custom/path/quiver.sock   Unix domain socket at custom path
  unix://                           Unix domain socket at default path (~/.quiver/quiver.sock; the named pipe on Windows)
  npipe://quiver                    Windows named pipe (\\.\pipe\quiver)
  tcp://0.0.0.0:40257               TCP socket (remote mode)`)

	return cmd
}

// scopeDevHome points QUIVER_HOME at a .quiver directory inside the current
// working directory when running an unstamped dev build — the default
// whenever the binary wasn't built through make build's -ldflags, i.e.
// `go run ./cmd/quiver daemon`. This keeps a local run's state (events,
// store, vault cache, config.yaml, logs) out of the real ~/.quiver a release
// build uses, so it can never share or corrupt that state. A QUIVER_HOME the
// caller already set is left untouched.
func scopeDevHome(binVersion string) error {
	if binVersion != "dev" {
		return nil
	}
	if _, ok := os.LookupEnv("QUIVER_HOME"); ok {
		return nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("scope dev home: %w", err)
	}

	if err := os.Setenv("QUIVER_HOME", filepath.Join(cwd, ".quiver")); err != nil {
		return fmt.Errorf("scope dev home: %w", err)
	}
	return nil
}
