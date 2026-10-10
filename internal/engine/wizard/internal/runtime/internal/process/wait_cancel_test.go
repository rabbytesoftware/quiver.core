package process

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/runtime/internal/models"
)

func longRunning() string {
	if runtime.GOOS == "windows" {
		return "ping -n 60 127.0.0.1 > nul"
	}
	return "sleep 60"
}

func TestWait_CancelledContextReturnsOnlyOnceTheProcessIsGone(t *testing.T) {
	config := models.NewConfig([]string{longRunning()})
	config.ShellWrap = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proc, err := newProcess(ctx, config)
	if err != nil {
		t.Fatalf("newProcess() error = %v", err)
	}
	t.Cleanup(func() { _ = proc.Close() })
	pid := proc.PID()

	waited := make(chan error, 1)
	go func() { waited <- proc.Wait(ctx) }()
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-waited:
		if err != context.Canceled {
			t.Fatalf("Wait() error = %v, want context.Canceled", err)
		}
	case <-time.After(cancelGrace + 5*time.Second):
		t.Fatal("Wait() did not return after the context was cancelled")
	}

	select {
	case <-proc.Done():
	default:
		t.Fatal("Wait() returned while the process was still running")
	}
	if isAlive(pid) {
		t.Fatalf("process %d still alive after Wait() returned", pid)
	}
}
