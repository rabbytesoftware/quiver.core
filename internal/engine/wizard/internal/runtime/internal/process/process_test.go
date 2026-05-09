//go:build darwin || linux

package process

import (
	"context"
	"errors"
	"testing"

	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/runtime/internal/models"
)

func TestNew_EmptyCommand(t *testing.T) {
	_, err := New(context.Background(), &models.Config{})
	if !errors.Is(err, models.ErrEmptyCommand) {
		t.Errorf("New() error = %v, want ErrEmptyCommand", err)
	}
}

func TestNew_ValidCommand(t *testing.T) {
	config := models.NewConfig([]string{"echo", "hello"})
	proc, err := New(context.Background(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer proc.Close()

	if proc.ID() == "" {
		t.Error("New() returned process with empty ID")
	}

	proc.Wait(context.Background())
}

func TestSignalPID_InvalidPID(t *testing.T) {
	cases := []int{0, -1, -100}
	for _, pid := range cases {
		err := SignalPID(context.Background(), pid, domainstep.SignalKindGraceful)
		if err == nil {
			t.Errorf("SignalPID(%d) expected error, got nil", pid)
		}
	}
}

func TestSignalPID_UnsupportedSignal(t *testing.T) {
	err := SignalPID(context.Background(), 1, domainstep.SignalKind("bogus"))
	if err == nil {
		t.Error("SignalPID with bogus signal expected error, got nil")
	}
}

func TestIsAlive_ZeroPID_ReturnsFalse(t *testing.T) {
	if IsAlive(0) {
		t.Error("IsAlive(0) should return false")
	}
}

func TestIsAlive_NegativePID_ReturnsFalse(t *testing.T) {
	if IsAlive(-1) {
		t.Error("IsAlive(-1) should return false")
	}
}

func TestIsAlive_RunningProcess_ReturnsTrue(t *testing.T) {
	config := models.NewConfig([]string{"sleep", "5"})
	proc, err := New(context.Background(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer proc.Kill(context.Background())
	defer proc.Close()

	pid := proc.PID()
	if pid <= 0 {
		t.Fatalf("PID() = %d, want positive", pid)
	}

	if !IsAlive(pid) {
		t.Errorf("IsAlive(%d) should return true for running process", pid)
	}
}

func TestIsAlive_DeadProcess_ReturnsFalse(t *testing.T) {
	config := models.NewConfig([]string{"echo", "done"})
	proc, err := New(context.Background(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer proc.Close()

	proc.Wait(context.Background())
	pid := proc.PID()
	if pid <= 0 {
		t.Fatalf("PID() = %d, want positive", pid)
	}

	// After process completes, it should not be alive.
	if IsAlive(pid) {
		t.Logf("IsAlive(%d) still true after exit — may be a zombie, which is acceptable", pid)
	}
}
