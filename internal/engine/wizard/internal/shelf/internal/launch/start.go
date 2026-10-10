package launch

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrOpenFailed means open(1) ran and refused the bundle.
var ErrOpenFailed = errors.New("launch: open refused the bundle")

var (
	openCommand = "/usr/bin/open"
	bundleHost  = runtime.GOOS == "darwin"
	openWait    = 2 * time.Second
)

// IsBundle reports whether target is a macOS application bundle that open(1)
// starts, as opposed to a file or directory this host executes itself.
func IsBundle(
	target string,
) bool {
	return bundleHost && strings.EqualFold(filepath.Ext(target), ".app")
}

// Start launches target detached from the caller: it gets its own session or
// process group, outlives the daemon and is never waited on past the first
// moments. A macOS bundle goes through open(1), whose refusal is reported; any
// other target is executed from its own directory.
func Start(
	target string,
) error {
	err := spawn(target, true)
	if breakawayDenied(err) {
		err = spawn(target, false)
	}
	return err
}

func spawn(
	target string,
	breakaway bool,
) error {
	cmd := command(target)
	if !IsBundle(target) {
		cmd.Dir = filepath.Dir(target)
	}
	detach(cmd, target, breakaway)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch %s: %w", target, err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if !IsBundle(target) {
		return nil
	}
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%w: %s: %w", ErrOpenFailed, target, err)
		}
	case <-time.After(openWait):
	}
	return nil
}

func command(
	target string,
) *exec.Cmd {
	if IsBundle(target) {
		return exec.Command(openCommand, target) // #nosec G204 -- target is a path the shelf recorded and re-validated
	}
	return exec.Command(target) // #nosec G204 -- target is a path the shelf recorded and re-validated
}
