//go:build darwin || linux

package process

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/runtime/internal/models"
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
