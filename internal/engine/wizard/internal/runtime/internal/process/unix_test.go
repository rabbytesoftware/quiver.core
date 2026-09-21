//go:build darwin || linux

package process

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/runtime/internal/models"
)

func startTestProcess(
	t *testing.T,
	command string,
) Process {
	t.Helper()
	config := models.NewConfig([]string{command})
	config.ShellWrap = true
	proc, err := newProcess(context.Background(), config)
	if err != nil {
		t.Fatalf("newProcess() error = %v", err)
	}
	return proc
}

func TestUnixProcess_Stop(t *testing.T) {
	proc := startTestProcess(t, "sleep 10")
	defer proc.Close()

	err := proc.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
	if proc.Status() != models.StatusFinished {
		t.Errorf("Status = %v, want %v", proc.Status(), models.StatusFinished)
	}
}

func TestUnixProcess_Kill(t *testing.T) {
	proc := startTestProcess(t, "sleep 10")
	defer proc.Close()

	err := proc.Kill(context.Background())
	if err != nil {
		t.Errorf("Kill() error = %v", err)
	}
	if proc.Status() != models.StatusFinished {
		t.Errorf("Status = %v, want %v", proc.Status(), models.StatusFinished)
	}
}

func TestUnixProcess_Interrupt(t *testing.T) {
	proc := startTestProcess(t, "sleep 10")
	defer proc.Close()

	err := proc.Interrupt(context.Background())
	if err != nil {
		t.Errorf("Interrupt() error = %v", err)
	}
	if proc.Status() != models.StatusFinished {
		t.Errorf("Status = %v, want %v", proc.Status(), models.StatusFinished)
	}
}

func TestUnixProcess_Stop_InvalidState(t *testing.T) {
	proc := startTestProcess(t, "echo done")
	defer proc.Close()

	proc.Wait(context.Background())

	err := proc.Stop(context.Background())
	if !errors.Is(err, models.ErrInvalidState) {
		t.Errorf("Stop() on finished process = %v, want ErrInvalidState", err)
	}
}

func TestUnixProcess_StatusTransitions(t *testing.T) {
	proc := startTestProcess(t, "echo hello")
	defer proc.Close()

	if proc.Status() != models.StatusRunning {
		t.Errorf("initial Status = %v, want %v", proc.Status(), models.StatusRunning)
	}

	proc.Wait(context.Background())

	if proc.Status() != models.StatusFinished {
		t.Errorf("final Status = %v, want %v", proc.Status(), models.StatusFinished)
	}
}

func TestUnixProcess_OutputCapture(t *testing.T) {
	proc := startTestProcess(t, "echo hello")
	defer proc.Close()

	proc.Wait(context.Background())

	output := proc.Output()
	if output == "" {
		t.Error("Output() should not be empty after process completes")
	}
}

func TestUnixProcess_ExitCode(t *testing.T) {
	proc := startTestProcess(t, "exit 0")
	defer proc.Close()

	proc.Wait(context.Background())

	if code := proc.ExitCode(); code != 0 {
		t.Errorf("ExitCode() = %d, want 0", code)
	}
}

func TestUnixProcess_ExitCode_Nonzero(t *testing.T) {
	proc := startTestProcess(t, "exit 1")
	defer proc.Close()

	proc.Wait(context.Background())

	if code := proc.ExitCode(); code != 1 {
		t.Errorf("ExitCode() = %d, want 1", code)
	}
}

func TestUnixProcess_Done(t *testing.T) {
	proc := startTestProcess(t, "echo hello")
	defer proc.Close()

	proc.Wait(context.Background())

	select {
	case <-proc.Done():
	default:
		t.Error("Done() channel should be closed after process finishes")
	}
}

func TestUnixProcess_PID(t *testing.T) {
	proc := startTestProcess(t, "sleep 10")
	defer proc.Kill(context.Background())
	defer proc.Close()

	if proc.PID() <= 0 {
		t.Errorf("PID() = %d, want positive value", proc.PID())
	}
}

func TestEnvMapToSlice(t *testing.T) {
	env := map[string]string{"KEY": "value", "FOO": "bar"}
	result := envMapToSlice(env)

	if len(result) != 2 {
		t.Errorf("len = %d, want 2", len(result))
	}
	seen := make(map[string]bool)
	for _, kv := range result {
		seen[kv] = true
	}
	if !seen["KEY=value"] {
		t.Error("missing KEY=value")
	}
	if !seen["FOO=bar"] {
		t.Error("missing FOO=bar")
	}
}

func TestEnvMapToSlice_Nil(t *testing.T) {
	if result := envMapToSlice(nil); result != nil {
		t.Errorf("envMapToSlice(nil) = %v, want nil", result)
	}
}

func TestUnixProcess_WithCustomBufferSize(t *testing.T) {
	config := models.NewConfig([]string{"echo", "hello"})
	config.BufferSize = 50

	proc, err := newProcess(context.Background(), config)
	if err != nil {
		t.Fatalf("newProcess() error = %v", err)
	}
	defer proc.Close()

	proc.Wait(context.Background())

	if proc.Output() == "" {
		t.Error("Output() should not be empty")
	}
}

func TestUnixProcess_Stop_ZeroTimeout(t *testing.T) {
	config := models.NewConfig([]string{"sleep", "10"})
	config.StopTimeout = 0

	proc, err := newProcess(context.Background(), config)
	if err != nil {
		t.Fatalf("newProcess() error = %v", err)
	}
	defer proc.Close()

	err = proc.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop() with zero timeout = %v, want nil", err)
	}
}

func TestUnixProcess_Kill_ZeroTimeout(t *testing.T) {
	config := models.NewConfig([]string{"sleep", "10"})
	config.KillTimeout = 0

	proc, err := newProcess(context.Background(), config)
	if err != nil {
		t.Fatalf("newProcess() error = %v", err)
	}
	defer proc.Close()

	err = proc.Kill(context.Background())
	if err != nil {
		t.Errorf("Kill() with zero timeout = %v, want nil", err)
	}
}

func TestOutputHandler_ChannelFull(t *testing.T) {
	h := newOutputHandler(1, 1)

	h.writeOutput("first")
	h.writeOutput("second") // channel full, drop

	select {
	case line := <-h.outChan:
		if line != "first" {
			t.Errorf("got %q, want \"first\"", line)
		}
	default:
		t.Error("expected a line in outChan")
	}
}

func TestOutputHandler_ErrorChannelFull(t *testing.T) {
	h := newOutputHandler(1, 1)

	h.writeError("first")
	h.writeError("second") // channel full, drop

	select {
	case line := <-h.errChan:
		if line != "first" {
			t.Errorf("got %q, want \"first\"", line)
		}
	default:
		t.Error("expected a line in errChan")
	}
}

func TestOutputHandler_WriteAfterClose(t *testing.T) {
	h := newOutputHandler(1, 1)
	h.close()

	// Should not panic; writes go to buffer only.
	h.writeOutput("should be dropped")
	h.writeError("should be dropped")
}

func TestUnixProcess_StartCommon_InvalidBinary(t *testing.T) {
	config := models.NewConfig([]string{"/no/such/binary"})

	_, err := newProcess(context.Background(), config)
	if err == nil {
		t.Error("newProcess() with nonexistent binary expected error")
	}
}

func TestStartCommon_InvalidState(t *testing.T) {
	config := models.NewConfig([]string{"echo", "hello"})
	base, err := newBaseProcess(context.Background(), config)
	if err != nil {
		t.Fatalf("newBaseProcess() error = %v", err)
	}
	base.status = models.StatusRunning // simulate already-started

	err = base.startCommon()
	if !errors.Is(err, models.ErrInvalidState) {
		t.Errorf("startCommon() = %v, want ErrInvalidState", err)
	}
}

func TestUnixProcess_StopWithTimeout_ContextCancelled(t *testing.T) {
	proc := startTestProcess(t, "sleep 10")
	defer proc.Kill(context.Background())
	defer proc.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	err := proc.Stop(ctx)
	if err == nil {
		t.Error("Stop() with cancelled context should return error")
	}
}

func TestStopWithTimeout_DeadlineExceeded(t *testing.T) {
	config := models.NewConfig([]string{"sleep", "10"})
	proc, err := newProcess(context.Background(), config)
	if err != nil {
		t.Fatalf("newProcess() error = %v", err)
	}
	defer proc.Kill(context.Background())
	defer proc.Close()

	p := proc.(*unixProcess)
	err = p.stopWithTimeout(
		context.Background(),
		5*time.Millisecond,
		func() error { return nil }, // no signal — process stays alive
		models.StatusStopping,
	)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Errorf("stopWithTimeout() = %v, want timeout error", err)
	}
}

func TestSignalPID_NonExistentPID(t *testing.T) {
	// PID 999999 almost certainly doesn't exist; expect ESRCH error.
	err := SignalPID(context.Background(), 999999, "graceful")
	if err == nil {
		t.Error("SignalPID(999999) expected error for nonexistent process, got nil")
	}
}

func TestStopWithTimeout_SendError(t *testing.T) {
	config := models.NewConfig([]string{"sleep", "10"})
	proc, err := newProcess(context.Background(), config)
	if err != nil {
		t.Fatalf("newProcess() error = %v", err)
	}
	defer proc.Kill(context.Background())
	defer proc.Close()

	p := proc.(*unixProcess)
	sendErr := errors.New("send failed")
	got := p.stopWithTimeout(
		context.Background(),
		5*time.Millisecond,
		func() error { return sendErr },
		models.StatusStopping,
	)
	if !errors.Is(got, sendErr) {
		t.Errorf("stopWithTimeout() = %v, want %v", got, sendErr)
	}
}

func TestStartCommon_ScannerContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	config := models.NewConfig([]string{"sh", "-c", "while true; do echo line; done"})
	proc, err := newProcess(ctx, config)
	if err != nil {
		t.Fatalf("newProcess() error = %v", err)
	}
	defer proc.Close()

	cancel()
	proc.Kill(context.Background())
	proc.Wait(context.Background())
}

func TestUnixProcess_ErrorCapture(t *testing.T) {
	proc := startTestProcess(t, "echo err >&2")
	defer proc.Close()

	proc.Wait(context.Background())

	if proc.Error() == "" {
		t.Error("Error() should not be empty when process writes to stderr")
	}
}

func TestUnixProcess_StreamOutput(t *testing.T) {
	proc := startTestProcess(t, "echo hello")
	defer proc.Close()

	var lines []string
	for line := range proc.StreamOutput() {
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		t.Error("StreamOutput() yielded no lines")
	}
}

func TestUnixProcess_StreamError(t *testing.T) {
	proc := startTestProcess(t, "echo err >&2")
	defer proc.Close()

	var lines []string
	for line := range proc.StreamError() {
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		t.Error("StreamError() yielded no lines")
	}
}

func TestUnixProcess_Wait_CancelledContext(t *testing.T) {
	proc := startTestProcess(t, "sleep 10")
	defer proc.Kill(context.Background())
	defer proc.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := proc.Wait(ctx)
	if err == nil {
		t.Error("Wait() with cancelled context should return error")
	}
}

func TestUnixProcess_Kill_AlreadyFinished(t *testing.T) {
	proc := startTestProcess(t, "echo done")
	defer proc.Close()

	proc.Wait(context.Background())

	// Kill on a finished process should not error (process gone is ok)
	err := proc.Kill(context.Background())
	if err != nil {
		t.Errorf("Kill() on finished process = %v, want nil", err)
	}
}

func TestSignalPID_Kill(t *testing.T) {
	proc := startTestProcess(t, "sleep 10")
	defer proc.Close()

	pid := proc.PID()
	if pid <= 0 {
		t.Fatalf("PID() = %d, want positive", pid)
	}

	err := SignalPID(context.Background(), pid, "kill")
	if err != nil {
		t.Errorf("SignalPID(kill) = %v, want nil", err)
	}
	proc.Wait(context.Background())
}

func TestSignalPID_Graceful(t *testing.T) {
	proc := startTestProcess(t, "sleep 10")
	defer proc.Close()

	pid := proc.PID()
	if pid <= 0 {
		t.Fatalf("PID() = %d, want positive", pid)
	}

	err := SignalPID(context.Background(), pid, "graceful")
	if err != nil {
		t.Errorf("SignalPID(graceful) = %v, want nil", err)
	}
	proc.Wait(context.Background())
}

func TestSignalPID_Interrupt(t *testing.T) {
	proc := startTestProcess(t, "sleep 10")
	defer proc.Close()

	pid := proc.PID()
	if pid <= 0 {
		t.Fatalf("PID() = %d, want positive", pid)
	}

	err := SignalPID(context.Background(), pid, "interrupt")
	if err != nil {
		t.Errorf("SignalPID(interrupt) = %v, want nil", err)
	}
	proc.Wait(context.Background())
}

// startForkingShellProcess starts a shell-wrapped step whose /bin/sh is forced
// to fork a child and stay alive supervising it, and returns the process
// alongside the wrapper's PID and the forked child's PID.
//
// `sleep 300` on its own — the command the self-update fixture arrow runs — is
// the case CI actually hit, but whether a shell forks for it or exec-optimizes
// itself into it is shell- and version-dependent (dash on ubuntu-latest forks;
// the /bin/sh on a developer's macOS does not). Backgrounding plus `wait`
// forces the fork on every POSIX shell, so the bug reproduces here wherever
// this suite runs rather than only on the runner that happened to ship dash.
func startForkingShellProcess(
	t *testing.T,
	ctx context.Context,
	killTimeout time.Duration,
) (Process, int, int) {
	t.Helper()

	config := models.NewConfig([]string{"sleep 300 & echo $!; wait"})
	config.ShellWrap = true
	config.KillTimeout = killTimeout
	config.StopTimeout = killTimeout

	proc, err := newProcess(ctx, config)
	if err != nil {
		t.Fatalf("newProcess() error = %v", err)
	}

	pid := proc.PID()
	if pid <= 0 {
		t.Fatalf("PID() = %d, want positive", pid)
	}
	// Never leak a 300-second sleep past this test, whichever assertion fails.
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = proc.Close()
	})

	var line string
	select {
	case line = <-proc.StreamOutput():
	case <-time.After(5 * time.Second):
		t.Fatal("shell never reported its background child's PID")
	}

	childPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("child PID %q: %v", line, err)
	}
	if childPID == pid {
		t.Fatalf("child PID %d equals wrapper PID %d: the shell did not fork", childPID, pid)
	}
	if !isAlive(childPID) {
		t.Fatalf("forked child %d is not alive: the premise of this test does not hold", childPID)
	}

	return proc, pid, childPID
}

// TestUnixProcess_Kill_ForkingShell_ReapsWrapperAndChild pins the CI failure
// this fix exists for. Killing only the wrapper's PID leaves the forked child
// running with the stdout/stderr pipe write-ends it inherited still open, so
// startCommon's scanners never reach EOF, cmd.Wait is never called, and the
// wrapper stays an unreaped zombie that isAlive — and therefore the wizard's
// ProcessAlive — reports as running forever.
//
// Both assertions below fail without the group-wide kill: Kill returns
// ErrKillTimeout because done never closes, and isAlive still answers true.
// Kill returning nil is itself the proof the child was reached: done closes
// only once every holder of those pipes is gone.
func TestUnixProcess_Kill_ForkingShell_ReapsWrapperAndChild(t *testing.T) {
	proc, pid, childPID := startForkingShellProcess(t, context.Background(), 5*time.Second)

	if err := proc.Kill(context.Background()); err != nil {
		t.Fatalf("Kill() error = %v, want nil (child %d kept the pipes open)", err, childPID)
	}
	if isAlive(pid) {
		t.Errorf("wrapper %d still reported alive after Kill: it was never reaped", pid)
	}
}

// TestSignalPID_ForkingShell_ReachesTheWholeGroup covers the same leak on the
// `signal` step path — the one a manifest's own `stop` lifecycle uses, and the
// path the self-update fixture arrow's stop step takes. A graceful signal that
// only reaches the wrapper shell leaves the step's real work running past what
// the user asked to stop.
func TestSignalPID_ForkingShell_ReachesTheWholeGroup(t *testing.T) {
	proc, pid, childPID := startForkingShellProcess(t, context.Background(), 5*time.Second)

	if err := SignalPID(context.Background(), pid, "graceful"); err != nil {
		t.Fatalf("SignalPID(graceful) = %v, want nil", err)
	}

	select {
	case <-proc.Done():
	case <-time.After(5 * time.Second):
		t.Fatalf("process never settled: child %d outlived the signal to wrapper %d", childPID, pid)
	}
	if isAlive(pid) {
		t.Errorf("wrapper %d still reported alive after a graceful signal", pid)
	}
}

// TestUnixProcess_ContextCancel_ForkingShell_TakesTheGroup covers the last
// place a spawned process is killed by PID alone: exec.CommandContext's own
// cancellation, which is how wizard.Shutdown ends a one-shot lifecycle method.
// Cancelling has to reach whatever the step's shell forked, or the work
// outlives the daemon that asked for it.
func TestUnixProcess_ContextCancel_ForkingShell_TakesTheGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	proc, pid, childPID := startForkingShellProcess(t, ctx, 5*time.Second)

	cancel()

	select {
	case <-proc.Done():
	case <-time.After(5 * time.Second):
		t.Fatalf("process never settled after cancel: child %d outlived wrapper %d", childPID, pid)
	}
	if isAlive(pid) {
		t.Errorf("wrapper %d still reported alive after its context was cancelled", pid)
	}
}

// TestSignalGroup_NonPositivePID guards the one input that must never reach
// syscall.Kill: kill(0, sig) addresses the caller's own process group, which
// would take down the daemon itself.
func TestSignalGroup_NonPositivePID(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if err := signalGroup(pid, syscall.SIGTERM); err == nil {
			t.Errorf("signalGroup(%d) expected error, got nil", pid)
		}
	}
}

func TestUnixProcess_WithEnv(t *testing.T) {
	config := models.NewConfig([]string{"sh", "-c", "echo $TEST_VAR"})
	config.Env["TEST_VAR"] = "hello_env"

	proc, err := newProcess(context.Background(), config)
	if err != nil {
		t.Fatalf("newProcess() error = %v", err)
	}
	defer proc.Close()

	proc.Wait(context.Background())

	if proc.Output() == "" {
		t.Error("Output() should contain env var output")
	}
}
