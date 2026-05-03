package run_test

import (
	"context"
	"os"
	"testing"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/models"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/runtime"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/step"
	steprun "github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/step/run"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testNSKey = "test/user/repo/arrow"

func newTestHandler(t *testing.T) wizstep.Handler[domainstep.RunStep] {
	t.Helper()
	rt, err := runtime.New()
	require.NoError(t, err)
	return steprun.NewHandler(rt)
}

func testReq() wizstep.Request {
	return wizstep.Request{
		NSKey:   testNSKey,
		WorkDir: os.TempDir(),
		Vars:    map[string]string{},
	}
}

func TestHandler_Execute_Success(t *testing.T) {
	h := newTestHandler(t)
	s := domainstep.NewRunStep("echo", "echo hello", false, "5s", true)

	err := h.Execute(context.Background(), testReq(), s)

	require.NoError(t, err)
}

func TestHandler_Execute_NonZeroExit(t *testing.T) {
	h := newTestHandler(t)
	s := domainstep.NewRunStep("fail", "false", false, "5s", true)

	err := h.Execute(context.Background(), testReq(), s)

	require.Error(t, err)
	assert.ErrorIs(t, err, steprun.ErrNonZeroExit)
}

func TestHandler_Execute_Timeout(t *testing.T) {
	h := newTestHandler(t)
	s := domainstep.NewRunStep("sleep", "sleep 10", false, "50ms", true)

	err := h.Execute(context.Background(), testReq(), s)

	require.Error(t, err)
}

func TestHandler_Execute_EmitsPIDEvent(t *testing.T) {
	h := newTestHandler(t)
	var emitted []models.Event
	req := wizstep.Request{
		NSKey:   testNSKey,
		WorkDir: os.TempDir(),
		Vars:    map[string]string{},
		Emit:    func(ev models.Event) { emitted = append(emitted, ev) },
	}
	s := domainstep.NewRunStep("echo", "echo hi", false, "5s", true)

	err := h.Execute(context.Background(), req, s)
	require.NoError(t, err)

	require.Len(t, emitted, 1)
	assert.Equal(t, models.EventKindPID, emitted[0].Kind)
	assert.Greater(t, emitted[0].PID, 0)
}

func TestHandler_Execute_ContextCancelled(t *testing.T) {
	h := newTestHandler(t)
	started := make(chan struct{}, 1)
	req := wizstep.Request{
		NSKey:   testNSKey,
		WorkDir: os.TempDir(),
		Vars:    map[string]string{},
		Emit: func(ev models.Event) {
			if ev.Kind == models.EventKindPID {
				started <- struct{}{}
			}
		},
	}
	s := domainstep.NewRunStep("sleep", "sleep 10", false, "30s", true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- h.Execute(ctx, req, s) }()

	timeout, timeoutCancel := context.WithTimeout(context.Background(), 2e9)
	defer timeoutCancel()
	select {
	case <-started:
	case <-timeout.Done():
		t.Fatal("process did not start within timeout")
	}

	cancel()
	require.Error(t, <-errCh)
}

func TestHandler_Execute_InvalidTimeout_ReturnsError(t *testing.T) {
	h := newTestHandler(t)
	s := domainstep.NewRunStep("echo", "echo hi", false, "bad-timeout", true)

	err := h.Execute(context.Background(), testReq(), s)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timeout")
}

func TestHandler_Execute_NilEmit_NoPanic(t *testing.T) {
	h := newTestHandler(t)
	s := domainstep.NewRunStep("echo", "echo hello", false, "5s", true)

	err := h.Execute(context.Background(), testReq(), s)

	require.NoError(t, err)
}
