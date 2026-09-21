package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal"
	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
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

			// stop, not a shutdown path of its own: an update that finishes
			// cancels the same context a SIGTERM would, so the daemon leaves
			// through the one graceful sequence either way.
			trigger := selfupdate.NewTrigger(stop)

			container, err := internal.New(ctx, version, buildID, internal.WithSelfUpdateTrigger(trigger))
			if err != nil {
				return err
			}

			slog.Info("starting quiver daemon", "version", version, "build", buildID)

			return succeedIfUpdated(trigger, container.Start(ctx, host))
		},
	}

	cmd.Flags().StringVar(&host, "host", "", `host URI to bind (overrides config). Examples:
  unix:///custom/path/quiver.sock   Unix domain socket at custom path
  unix://                           Unix domain socket at default path (~/.quiver/quiver.sock)
  tcp://0.0.0.0:40257               TCP socket (remote mode)`)

	return cmd
}

// succeedIfUpdated hands this process over to the binary quiver.core's own
// update lifecycle produced, if there is one. It runs only after Start has
// returned, so the listener is closed and every aggregate drained before the
// successor exists — the relaunch needs no stop of its own, and cannot race
// the daemon it replaces for the socket or the databases.
//
// startErr is carried through rather than dropped: the daemon may well have
// left Start on an error of its own and the successor is still the right
// thing to start, but the operator still needs to see why the old one stopped.
//
// An error back from Relaunch does not by itself mean the machine lost its
// daemon — Relaunch falls back to the build that was already running — but it
// always means the update did not take, which is why it is logged at error and
// returned rather than absorbed.
func succeedIfUpdated(
	trigger *selfupdate.Trigger,
	startErr error,
) error {
	if !trigger.Fired() {
		return startErr
	}

	slog.Info("quiver daemon: relaunching after self-update", "new_binary", trigger.NewBinaryPath())

	if err := trigger.Relaunch(); err != nil {
		slog.Error("quiver daemon: self-update handover failed", "err", err)
		return errors.Join(startErr, err)
	}

	return startErr
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
