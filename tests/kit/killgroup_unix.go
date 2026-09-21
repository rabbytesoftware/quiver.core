//go:build integration && (darwin || linux)

package kit

import "syscall"

// killProcessGroup force-kills the whole process group led by pid.
//
// Every PID a test hands this was spawned by the wizard's own process package,
// which puts each one in a group of its own (Setpgid with Pgid left at zero),
// so pid is always the group leader. Killing that PID alone is not enough: a
// shell-wrapped step whose /bin/sh forks instead of exec-optimizing itself
// leaves the forked child orphaned but still holding the stdout/stderr pipe
// write-ends it inherited, which keeps the shell unreaped — and an unreaped
// zombie answers kill(pid, 0), so ProcessAlive reports it running forever.
// See signalGroup in
// internal/engine/wizard/internal/runtime/internal/process/unix.go, the
// production fix this mirrors.
func killProcessGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
