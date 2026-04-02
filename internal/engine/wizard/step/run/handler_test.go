package run_test

import (
	"context"
	"os"
	"testing"
	"time"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/mocks"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
	steprun "github.com/rabbytesoftware/quiver/internal/engine/wizard/step/run"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testNSKey = "test/user/repo/arrow"

func newTestHandler(t *testing.T) (wizstep.Handler[domainstep.RunStep], *runtime.Runtime) {
	t.Helper()
	rt, err := runtime.New()
	require.NoError(t, err)
	return steprun.NewHandler(rt), rt
}

func testReq() wizstep.Request {
	return wizstep.Request{
		NSKey:   testNSKey,
		WorkDir: os.TempDir(),
		Vars:    map[string]string{},
		Tracker: &mocks.Tracker{},
	}
}

func TestHandler_Execute_Success(t *testing.T) {
	h, _ := newTestHandler(t)
	s := domainstep.NewRunStep("echo", "echo hello", 5*time.Second, true)

	err := h.Execute(context.Background(), testReq(), s)

	require.NoError(t, err)
}

func TestHandler_Execute_NonZeroExit(t *testing.T) {
	h, _ := newTestHandler(t)
	s := domainstep.NewRunStep("fail", "false", 5*time.Second, true)

	err := h.Execute(context.Background(), testReq(), s)

	require.Error(t, err)
	assert.ErrorIs(t, err, steprun.ErrNonZeroExit)
}

func TestHandler_Execute_Timeout(t *testing.T) {
	h, _ := newTestHandler(t)
	s := domainstep.NewRunStep("sleep", "sleep 10", 50*time.Millisecond, true)

	err := h.Execute(context.Background(), testReq(), s)

	require.Error(t, err)
}

func TestHandler_Execute_StoresProcess(t *testing.T) {
	h, _ := newTestHandler(t)
	tracker := &mocks.Tracker{}
	req := wizstep.Request{
		NSKey:   testNSKey,
		WorkDir: os.TempDir(),
		Vars:    map[string]string{},
		Tracker: tracker,
	}
	s := domainstep.NewRunStep("echo", "echo hi", 5*time.Second, true)

	err := h.Execute(context.Background(), req, s)
	require.NoError(t, err)

	proc, ok := tracker.GetProcess()
	require.True(t, ok, "process should be set after Execute")
	assert.NotNil(t, proc)
}

func TestHandler_Execute_ContextCancelled(t *testing.T) {
	h, _ := newTestHandler(t)
	tracker := &mocks.Tracker{}
	req := wizstep.Request{
		NSKey:   testNSKey,
		WorkDir: os.TempDir(),
		Vars:    map[string]string{},
		Tracker: tracker,
	}
	s := domainstep.NewRunStep("sleep", "sleep 10", 30*time.Second, true)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- h.Execute(ctx, req, s)
	}()

	require.Eventually(t, func() bool {
		_, ok := tracker.GetProcess()
		return ok
	}, 2*time.Second, 10*time.Millisecond)

	cancel()
	err := <-errCh
	require.Error(t, err)
}

func TestHandler_Execute_FinishedProcessCleanedUp(t *testing.T) {
	// Run a command that exits non-zero. The handler uses ShellWrap so sh starts
	// successfully, runs the command, and exits. The process is StatusFinished
	// after Execute returns. CleanupFinished should remove it from the manager.
	h, rt := newTestHandler(t)
	s := domainstep.NewRunStep("bad", "false", 5*time.Second, true)

	err := h.Execute(context.Background(), testReq(), s)
	require.Error(t, err)

	// Process is registered and in StatusFinished at this point.
	assert.Equal(t, 1, rt.Count(), "process should still be registered after handler returns")

	cleaned := rt.CleanupFinished()
	assert.Equal(t, 1, cleaned, "CleanupFinished should remove the finished process")
	assert.Equal(t, 0, rt.Count(), "manager should be empty after cleanup")
}
