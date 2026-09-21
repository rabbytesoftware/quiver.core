//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// sigpipeBuffer is 1, and nothing ever reads the channel. os/signal sends
// non-blockingly, so a full buffer drops the signal — which is precisely the
// wanted outcome. A goroutine draining it would do the same work and add a
// lifetime to manage.
const sigpipeBuffer = 1

// surviveBrokenLogPipe stops a write to a closed stdout or stderr from killing
// this process, and returns the function that undoes it.
//
// Go's runtime terminates a program that takes SIGPIPE from a write to file
// descriptor 1 or 2 — deliberately, so that `quiver arrow list | head` behaves
// like every other unix command. For the DAEMON that default is wrong, and
// this E2E pass is what proved it is wrong in practice rather than in theory.
//
// quiver.desktop starts its sidecar daemon through tauri-plugin-shell, which
// hands the child pipes so the app can pump the daemon's output into its own
// log (SidecarManager::spawn -> pump_events). The app is therefore the only
// reader of those pipes. quiver.desktop's own update lifecycle then kills the
// app by design — `pkill -x quiverdesktop`, chosen so the app cannot reach
// Tauri's ExitRequested handler and reap the very daemon running the update —
// and the plan is that the daemon is reparented and carries on to finish.
// What actually happened is that the app's death closed the read end of both
// pipes, the daemon's next log line raised SIGPIPE, and the daemon was killed
// by the runtime mid-update: old app gone, new one never placed, no daemon
// left to place it.
//
// os/signal documents this exact escape hatch: once a program calls Notify for
// SIGPIPE, SIGPIPE is delivered to the channel instead of terminating the
// program, wherever it came from. The write then fails with EPIPE, which is
// what a log write should do when nobody is listening.
//
// Scoped to the daemon command on purpose. A one-shot CLI invocation piped
// into `head` SHOULD still die on a broken pipe; a background service that
// outlives whoever started it must not.
func surviveBrokenLogPipe() func() {
	ch := make(chan os.Signal, sigpipeBuffer)
	signal.Notify(ch, syscall.SIGPIPE)

	return func() { signal.Stop(ch) }
}
