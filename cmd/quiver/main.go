package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands"
	"github.com/rabbytesoftware/quiver.core/internal/cli/daemon"
)

// Injected at build time via -ldflags.
// buildID is days elapsed since the Quiver epoch (2026-04-11 15:33:00 ART / 18:33:00 UTC).
var (
	version = "dev"
	buildID = "0"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "quiver",
		Version:       fmt.Sprintf("%s (build %s)", version, buildID),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newDaemonCmd())
	commands.Attach(cmd, newCLIDeps())
	return cmd
}

func main() {
	root := newRootCmd()
	err := root.Execute()

	if shouldManageDaemon(os.Args[1:]) {
		if mgr, mgrErr := daemon.NewManager(); mgrErr == nil {
			stopIdleDaemon(context.Background(), mgr)
		}
	}

	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "quiver:", err)
		os.Exit(commands.ExitCode(err))
	}
}
