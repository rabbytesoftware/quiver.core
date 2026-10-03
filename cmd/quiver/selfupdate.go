package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/app/selfarrow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/daemon"
	"github.com/rabbytesoftware/quiver.core/internal/cli/selfupdate"
	"github.com/rabbytesoftware/quiver.core/internal/core/gateway"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
)

var errNotAFile = errors.New("not a regular file")

// selfUpdateDeps are the process-level collaborators of the self-update
// command, replaced by fakes in tests.
type selfUpdateDeps struct {
	executable func() (string, error)
	detach     func(exe string, args ...string) error
	run        func(ctx context.Context, newBin string) error
}

func realSelfUpdateDeps() selfUpdateDeps {
	return selfUpdateDeps{
		executable: os.Executable,
		detach:     selfupdate.Detach,
		run:        runSelfUpdate,
	}
}

// newSelfUpdateCmd is how quiver.core's own arrow updates itself: the update
// downloads the new binary and runs this command on it. Run by the daemon it
// is about to replace, it only starts a detached copy of itself and returns,
// so the update run ends normally; the copy does the swap.
func newSelfUpdateCmd(deps selfUpdateDeps) *cobra.Command {
	var detached bool

	cmd := &cobra.Command{
		Use:    "self-update <new-binary>",
		Short:  "Replace the running daemon with a newer binary",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			newBin, err := regularFile(args[0])
			if err != nil {
				return err
			}
			if detached {
				return deps.run(cmd.Context(), newBin)
			}

			exe, err := deps.executable()
			if err != nil {
				return fmt.Errorf("self-update: locate executable: %w", err)
			}
			return deps.detach(exe, "self-update", "--detached", newBin)
		},
	}

	cmd.Flags().BoolVar(&detached, "detached", false, "do the swap in this process")
	_ = cmd.Flags().MarkHidden("detached")

	return cmd
}

func regularFile(arg string) (string, error) {
	abs, err := filepath.Abs(arg)
	if err != nil {
		return "", fmt.Errorf("self-update: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("self-update: new binary: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("self-update: new binary %s: %w", abs, errNotAFile)
	}
	return abs, nil
}

// runSelfUpdate does the swap for the daemon on the default local socket. This
// process has no stdio, so its steps go to a log file beside the daemon's.
func runSelfUpdate(ctx context.Context, newBin string) error {
	if logs, err := paths.Logs(); err == nil {
		if f, err := os.OpenFile(filepath.Join(logs, "self-update.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil { // #nosec G304 -- a fixed name under the logs dir
			defer f.Close() //nolint:errcheck // log file
			slog.SetDefault(slog.New(slog.NewTextHandler(f, nil)))
		}
	}

	self, err := selfarrow.BinaryPath("")
	if err != nil {
		return fmt.Errorf("self-update: %w", err)
	}
	socket := gateway.LocalSocket(metadata.GetHomePath())

	updater, err := selfupdate.New(socket, self)
	if err != nil {
		return err
	}

	// Stop and the CLI signal the pid in quiver.pid; the daemon this run starts
	// is not the one that file names.
	if m, err := daemon.NewManager(); err == nil {
		updater.Started = func(pid int) {
			if err := m.RecordPID(pid); err != nil {
				slog.WarnContext(ctx, "self-update: record daemon pid", "err", err)
			}
		}
	}

	err = updater.Run(ctx, newBin)
	if err != nil {
		slog.ErrorContext(ctx, "self-update failed", "err", err)
	}
	return err
}
