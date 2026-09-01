package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
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
	cmd.AddCommand(newAuthCmd())
	return cmd
}

func main() {
	root := newRootCmd()

	if err := root.Execute(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
