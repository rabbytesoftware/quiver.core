//go:build integration && (darwin || linux)

package kit

import "syscall"

// killProcessGroup force-kills the whole process group led by pid: a
// shell-wrapped step that forks instead of exec-optimizing can leave an
// orphaned, unreaped child that keeps ProcessAlive reporting it running
// forever if only the leader is killed. Mirrors signalGroup in
// internal/engine/wizard/internal/runtime/internal/process/unix.go.
func killProcessGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
