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

// surviveBrokenLogPipe stops a write to a closed stdout or stderr from
// killing this process, and returns the function that undoes it.
//
// Go's runtime terminates a program on SIGPIPE from a write to fd 1 or 2 --
// deliberate, so `quiver arrow list | head` behaves like any unix command.
// For the daemon that default is wrong: quiver.desktop's update lifecycle
// kills its own sidecar app by design so the app cannot reap the daemon
// mid-update, but that kill also closes the read end of the daemon's own
// stdout/stderr pipes (tauri-plugin-shell hands them to the app). The
// daemon's next log line then raised SIGPIPE and killed it mid-update --
// found via the real E2E rig, not in theory.
//
// Notify redirects the signal to the channel instead of terminating the
// process; the write then fails with EPIPE, as a log write should when
// nobody is listening. Scoped to the daemon command only: a one-shot CLI
// piped into `head` should still die on a broken pipe.
func surviveBrokenLogPipe() func() {
	ch := make(chan os.Signal, sigpipeBuffer)
	signal.Notify(ch, syscall.SIGPIPE)

	return func() { signal.Stop(ch) }
}
