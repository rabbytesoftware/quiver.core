//go:build darwin || linux

package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	handOverChildEnv  = "QUIVER_TEST_SELFUPDATE_CHILD"
	handOverTargetEnv = "QUIVER_TEST_SELFUPDATE_TARGET"
)

// handOver really does replace the process, so the only honest test of it runs
// in a process this one can afford to lose: the test binary re-execs itself,
// the child hands over to a successor script, and the parent reads back what
// the successor printed. The successor is written non-executable on purpose —
// that is exactly how the fetch step leaves the downloaded binary, and a
// handover that skipped the chmod would fail here with EACCES.
func TestHandOver_ReplacesTheProcessImage(t *testing.T) {
	if os.Getenv(handOverChildEnv) == "1" {
		runHandOverChild()
		return
	}

	successor := filepath.Join(t.TempDir(), "quiver-new")
	require.NoError(t, os.WriteFile(successor, []byte("#!/bin/sh\necho SUCCESSOR-RAN\n"), 0o644))

	cmd := exec.Command(os.Args[0], "-test.run=TestHandOver_ReplacesTheProcessImage")
	cmd.Env = append(
		os.Environ(),
		handOverChildEnv+"=1",
		handOverTargetEnv+"="+successor,
	)

	out, err := cmd.CombinedOutput()

	require.NoError(t, err, "child: %s", out)
	assert.Contains(t, string(out), "SUCCESSOR-RAN")
	assert.NotContains(t, string(out), "CHILD-SURVIVED-EXEC")
}

func runHandOverChild() {
	if err := handOver(os.Getenv(handOverTargetEnv)); err != nil {
		fmt.Fprintln(os.Stderr, "handOver:", err)
		os.Exit(2)
	}
	fmt.Println("CHILD-SURVIVED-EXEC")
	os.Exit(3)
}

func TestHandOver_MissingBinary_ReturnsError(t *testing.T) {
	err := handOver(filepath.Join(t.TempDir(), "not-downloaded"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "selfupdate: relaunch: chmod")
}

func TestHandOver_UnreadableTarget_ReturnsExecError(t *testing.T) {
	target := filepath.Join(t.TempDir(), "quiver-new")
	require.NoError(t, os.WriteFile(target, []byte("not an executable format"), 0o644))

	err := handOver(target)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "selfupdate: relaunch: exec")
}
