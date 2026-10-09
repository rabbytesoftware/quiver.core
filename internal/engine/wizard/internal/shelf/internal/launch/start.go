package launch

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Start launches target detached from the caller: it gets its own session or
// process group and is not waited on, so it outlives the daemon. A macOS
// bundle goes through open(1); anything else is executed from its own
// directory.
func Start(
	target string,
) error {
	cmd := command(target)
	if !strings.HasSuffix(target, ".app") {
		cmd.Dir = filepath.Dir(target)
	}
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch %s: %w", target, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func command(
	target string,
) *exec.Cmd {
	if strings.HasSuffix(target, ".app") {
		return exec.Command("open", target) // #nosec G204 -- target is a path the shelf recorded and re-validated
	}
	return exec.Command(target) // #nosec G204 -- target is a path the shelf recorded and re-validated
}
