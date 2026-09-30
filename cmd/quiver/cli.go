package main

import (
	"context"
	"os"

	"github.com/charmbracelet/x/term"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands"
	"github.com/rabbytesoftware/quiver.core/internal/cli/daemon"
)

// newCLIDeps wires the command tree to the real process environment.
func newCLIDeps() commands.Deps {
	return commands.Deps{
		Version: version,
		IsTTY:   stdoutIsTTY,
		EnsureDaemon: func(ctx context.Context) error {
			mgr, err := daemon.NewManager()
			if err != nil {
				return err
			}
			return mgr.Ensure(ctx)
		},
	}
}

func stdoutIsTTY() bool {
	return fileIsTTY(os.Stdout)
}

// fileIsTTY asks the terminal itself: a character device such as /dev/null
// is no terminal, and handing it to the interactive renderer fails the
// command.
func fileIsTTY(f *os.File) bool {
	return term.IsTerminal(f.Fd())
}

// shouldManageDaemon reports whether the invocation may stop an idle local
// daemon afterwards. Running the daemon itself is exempt.
func shouldManageDaemon(args []string) bool {
	for _, arg := range args {
		if arg == "daemon" {
			return false
		}
		if len(arg) > 0 && arg[0] != '-' {
			return true
		}
	}
	return true
}

// stopIdleDaemon terminates a CLI-booted local daemon once nothing is
// active, honoring the promise that the daemon shuts down on its own. An
// update still settling counts as active: its steps ended, but the commit
// that advances its row has not landed yet.
// Every step is best-effort: a daemon that cannot be probed is left alone.
func stopIdleDaemon(ctx context.Context, mgr *daemon.Manager) {
	if _, err := mgr.ReadPID(); err != nil {
		return
	}
	if !mgr.IsLive() {
		return
	}

	cli, err := client.New("unix://" + mgr.Socket)
	if err != nil {
		return
	}
	runtimes, err := cli.ListRuntimes(ctx)
	if err != nil {
		return
	}
	for _, rt := range runtimes {
		if commands.IsActiveState(rt.State) || rt.Settling {
			return
		}
	}
	_ = mgr.Stop()
}
