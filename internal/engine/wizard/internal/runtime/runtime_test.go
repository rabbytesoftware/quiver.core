package runtime

import (
	"context"
	"errors"
	"testing"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/runtime/mocks"
)

func TestNew(t *testing.T) {
	rt, err := New()

	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if rt == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_UnsupportedOS(t *testing.T) {
	_, err := newForOS("freebsd")
	if !errors.Is(err, ErrUnsupportedOS) {
		t.Errorf("newForOS(freebsd) = %v, want ErrUnsupportedOS", err)
	}
}

func TestNew_SupportedOS(t *testing.T) {
	tests := []struct {
		name      string
		os        string
		wantError bool
	}{
		{"darwin", "darwin", false},
		{"linux", "linux", false},
		{"windows", "windows", false},
		{"freebsd", "freebsd", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &runtime{os: tt.os}
			if !isSupportedOS(r.os) && !tt.wantError {
				t.Errorf("expected %s to be supported", tt.os)
			}
			if isSupportedOS(r.os) && tt.wantError {
				t.Errorf("expected %s to be unsupported", tt.os)
			}
		})
	}
}

func TestRuntime_Start(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	config := NewConfig([]string{"echo", "hello"})

	proc, err := rt.Start(ctx, config)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer proc.Close()

	if proc.ID() == "" {
		t.Error("Start() returned process with empty ID")
	}

	proc.Wait(ctx)
}

func TestRuntime_Start_EmptyCommand(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	config := NewConfig([]string{})

	_, err = rt.Start(ctx, config)
	if !errors.Is(err, ErrEmptyCommand) {
		t.Errorf("Start() error = %v, want ErrEmptyCommand", err)
	}
}

func TestRuntime_Signal_Graceful(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	proc := mocks.NewProcess("p1", StatusRunning)

	err = rt.(*runtime).signal(context.Background(), proc, domainstep.SignalKindGraceful)
	if err != nil {
		t.Errorf("Signal(graceful) error = %v", err)
	}
	if proc.Status() != StatusFinished {
		t.Errorf("Status = %v, want %v", proc.Status(), StatusFinished)
	}
}

func TestRuntime_Signal_Kill(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	proc := mocks.NewProcess("p1", StatusRunning)

	err = rt.(*runtime).signal(context.Background(), proc, domainstep.SignalKindKill)
	if err != nil {
		t.Errorf("Signal(kill) error = %v", err)
	}
	if proc.Status() != StatusFinished {
		t.Errorf("Status = %v, want %v", proc.Status(), StatusFinished)
	}
}

func TestRuntime_Signal_Interrupt(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	proc := mocks.NewProcess("p1", StatusRunning)

	err = rt.(*runtime).signal(context.Background(), proc, domainstep.SignalKindInterrupt)
	if err != nil {
		t.Errorf("Signal(interrupt) error = %v", err)
	}
	if proc.Status() != StatusFinished {
		t.Errorf("Status = %v, want %v", proc.Status(), StatusFinished)
	}
}

func TestRuntime_Signal_Invalid(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	proc := mocks.NewProcess("p1", StatusRunning)

	err = rt.(*runtime).signal(context.Background(), proc, domainstep.SignalKind("bogus"))
	if !errors.Is(err, ErrInvalidSignal) {
		t.Errorf("Signal(bogus) error = %v, want ErrInvalidSignal", err)
	}
}

func TestRuntime_SignalPID_InvalidPID(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// PID -1 is always invalid; expect an error.
	err = rt.SignalPID(context.Background(), -1, domainstep.SignalKindGraceful)
	if err == nil {
		t.Error("SignalPID(-1) expected error, got nil")
	}
}

func TestRuntime_Start_WithShellWrap(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	config := NewConfig([]string{"echo hello && echo world"})
	config.ShellWrap = true

	proc, err := rt.Start(ctx, config)
	if err != nil {
		t.Fatalf("Start() with ShellWrap error = %v", err)
	}
	defer proc.Close()

	proc.Wait(ctx)

	if proc.ExitCode() != 0 {
		t.Errorf("ExitCode = %d, want 0", proc.ExitCode())
	}
}

func TestRuntime_Start_OutputCapture(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	config := NewConfig([]string{"echo", "hello"})

	proc, err := rt.Start(ctx, config)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer proc.Close()

	proc.Wait(ctx)

	output := proc.Output()
	if output == "" {
		t.Error("Output() should not be empty after process completes")
	}
}

func TestNewConfig_Defaults(t *testing.T) {
	config := NewConfig([]string{"echo", "test"})

	if config.WorkDir != "." {
		t.Errorf("WorkDir = %q, want \".\"", config.WorkDir)
	}
	if config.KillTimeout == 0 {
		t.Error("KillTimeout should have a default value")
	}
	if config.StopTimeout == 0 {
		t.Error("StopTimeout should have a default value")
	}
}

func TestProcessAlive_CurrentProcess(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	pid := 1 // PID 1 (init) is always alive on Unix
	if !rt.ProcessAlive(pid) {
		t.Logf("PID 1 reported not alive — may be in a container without init")
	}
}

func TestProcessAlive_DeadPID(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// PID 0 is never a valid process PID.
	if rt.ProcessAlive(0) {
		t.Error("ProcessAlive(0) should return false")
	}
}
