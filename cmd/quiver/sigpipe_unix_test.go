//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brokenPipeModeEnv switches this test binary into the child role used by both
// tests below. A subprocess is the only honest way to test this: the thing
// under test is whether a process SURVIVES, and a failure kills whoever runs
// it — which, in-process, would be the test binary itself.
const brokenPipeModeEnv = "QUIVER_TEST_BROKEN_PIPE_MODE"

// TestMain doubles as the child entry point. Falls through to the normal test
// run whenever the env var is absent, which is every ordinary invocation.
func TestMain(m *testing.M) {
	switch os.Getenv(brokenPipeModeEnv) {
	case "guarded":
		runBrokenPipeChild(true)
	case "bare":
		runBrokenPipeChild(false)
	}
	os.Exit(m.Run())
}

// runBrokenPipeChild writes to stdout long after its reader has gone. With the
// guard installed it must reach the exit below; without it, the Go runtime
// kills it with SIGPIPE partway through.
func runBrokenPipeChild(guarded bool) {
	if guarded {
		defer surviveBrokenLogPipe()()
	}

	// The parent closes its read end as soon as it sees this line, so the
	// writes after it land on a pipe with no reader.
	fmt.Println("ready")

	for i := 0; i < 200; i++ {
		fmt.Println("a log line nobody is reading any more")
		time.Sleep(time.Millisecond)
	}

	os.Exit(7)
}

// runChildLosingItsStdoutReader starts this binary in the given mode, waits
// for its "ready" line, closes the pipe, and reports how it ended.
func runChildLosingItsStdoutReader(t *testing.T, mode string) (exitCode int, signal syscall.Signal) {
	t.Helper()

	self, err := os.Executable()
	require.NoError(t, err)

	cmd := exec.Command(self) // #nosec G204 -- this test binary's own path
	cmd.Env = append(os.Environ(), brokenPipeModeEnv+"="+mode)

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())

	buf := make([]byte, len("ready\n"))
	_, readErr := stdout.Read(buf)
	require.NoError(t, readErr, "the child must announce itself before its reader goes away")

	// The reader going away is the whole experiment.
	require.NoError(t, stdout.Close())

	waitErr := cmd.Wait()

	status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	require.True(t, ok, "wait status: %v", waitErr)
	if status.Signaled() {
		return -1, status.Signal()
	}
	return status.ExitStatus(), 0
}

// TestSurviveBrokenLogPipe_ProcessOutlivesItsLogReader is the regression this
// whole guard exists for: quiver.desktop's update lifecycle kills the app that
// is reading its sidecar daemon's stdout, and the daemon has to stay up long
// enough to finish the update that killed it.
func TestSurviveBrokenLogPipe_ProcessOutlivesItsLogReader(t *testing.T) {
	code, sig := runChildLosingItsStdoutReader(t, "guarded")

	assert.Zero(t, sig, "the guarded child must not be killed by a signal")
	assert.Equal(t, 7, code, "the guarded child must run to its own exit")
}

// TestSurviveBrokenLogPipe_WithoutTheGuardTheProcessDies pins the default this
// guard overrides, so the test above cannot quietly start passing for the
// wrong reason if the guard is ever removed.
func TestSurviveBrokenLogPipe_WithoutTheGuardTheProcessDies(t *testing.T) {
	code, sig := runChildLosingItsStdoutReader(t, "bare")

	assert.Equal(t, syscall.SIGPIPE, sig,
		"without the guard the runtime kills a writer whose stdout reader has gone (exit code %s)",
		strconv.Itoa(code))
}

// TestSurviveBrokenLogPipe_StopIsSafeToCall covers the returned undo function,
// which the daemon command defers.
func TestSurviveBrokenLogPipe_StopIsSafeToCall(t *testing.T) {
	stop := surviveBrokenLogPipe()
	require.NotNil(t, stop)

	assert.NotPanics(t, stop)
	assert.NotPanics(t, stop, "a second call must be harmless: it is deferred, and a panic on the way out would mask the real error")
}

// TestSurviveBrokenLogPipe_DeliversRatherThanTerminates checks the mechanism
// directly: with the guard installed, a SIGPIPE aimed at this process is
// handled rather than fatal.
func TestSurviveBrokenLogPipe_DeliversRatherThanTerminates(t *testing.T) {
	defer surviveBrokenLogPipe()()

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGPIPE))

	// Reaching here at all is the assertion; the process would be gone
	// otherwise.
	assert.True(t, strings.HasPrefix(t.Name(), "TestSurviveBrokenLogPipe"))
}
