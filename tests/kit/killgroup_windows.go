//go:build integration && windows

package kit

import "os"

// killProcessGroup force-kills pid. Windows has no kill(-pgid) equivalent and
// windowsProcess never puts a spawned process into a group of its own, so the
// single process is all there is to reach — the same scope the wizard's own
// Windows signal path uses.
func killProcessGroup(pid int) {
	if pid <= 0 {
		return
	}
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Kill()
	}
}
