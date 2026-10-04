package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands"
	"github.com/rabbytesoftware/quiver.core/internal/cli/daemon"
)

// exitInternalError is the status a panic exits with. It matches the generic
// failure code: from a user's side an internal error is simply a failure.
const exitInternalError = 1

// Injected at build time via -ldflags.
// version is the release ref the build is published under, and commit the full
// hash it was built from; the two differ for a rolling tag, whose ref stays put
// while its commit moves. channel is the selector the release pipeline
// publishes the build under (stable, beta, hotfix, nightly-latest), empty for
// a local build.
// buildID is days elapsed since the Quiver epoch (2026-04-11 15:33:00 ART / 18:33:00 UTC).
var (
	version = "dev"
	commit  = ""
	channel = ""
	buildID = "0"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "quiver",
		Version:       fmt.Sprintf("%s (build %s)", version, buildID),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newDaemonCmd(), newSelfUpdateCmd(realSelfUpdateDeps()))
	commands.New(newCLIDeps()).Attach(cmd)
	return cmd
}

// recoverPanic converts any panic into the same single-line report every other
// failure gets. Several dependencies panic where they could return an error —
// yaml.v3 does so on a type it cannot marshal — and a stack trace dumped into a
// user's terminal is never the right answer. Set QUIVER_DEBUG to see the stack.
func recoverPanic() {
	r := recover()
	if r == nil {
		return
	}

	_, _ = fmt.Fprintln(os.Stderr, "quiver: internal error:", r)

	if os.Getenv("QUIVER_DEBUG") != "" {
		debug.PrintStack()
	}

	os.Exit(exitInternalError)
}

func main() {
	defer recoverPanic()

	root := newRootCmd()
	executed, err := root.ExecuteC()

	if shouldManageDaemon(os.Args[1:]) && !commands.IsLifecycle(executed) {
		if mgr, mgrErr := daemon.NewManager(); mgrErr == nil {
			stopIdleDaemon(context.Background(), mgr)
		}
	}

	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "quiver:", err)
		os.Exit(commands.ExitCode(err))
	}
}
