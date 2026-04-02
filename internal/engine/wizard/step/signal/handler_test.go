//go:build !windows

package signal

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testTracker is a minimal ProcessTracker for use in tests.
type testTracker struct {
	mu  sync.RWMutex
	key string
	set bool
}

func (t *testTracker) SetKey(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.key = key
	t.set = true
}

func (t *testTracker) GetKey() (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.key, t.set
}

func trackerWithKey(key string) wizstep.ProcessTracker {
	tr := &testTracker{}
	tr.SetKey(key)
	return tr
}

func newTestSetup(t *testing.T) (*handler, *runtime.Runtime) {
	t.Helper()
	rt, err := runtime.New()
	require.NoError(t, err)
	return NewHandler(rt).(*handler), rt
}

func testNSKey() string {
	return domain.Namespace("test/user/repo/arrow").String()
}

func startProcess(t *testing.T, rt *runtime.Runtime, command string) (key string) {
	t.Helper()
	proc, err := rt.Get(context.Background(), command).WithShellWrap().Build()
	require.NoError(t, err)
	require.NoError(t, proc.Start(context.Background()))
	return proc.Key()
}

func TestHandler_ShouldExecute(t *testing.T) {
	h, _ := newTestSetup(t)
	assert.True(t, h.ShouldExecute(domainstep.StepTypeSignal))
	assert.False(t, h.ShouldExecute(domainstep.StepTypeRun))
	assert.False(t, h.ShouldExecute(domainstep.StepTypeFetch))
}

func TestHandler_Execute_Success(t *testing.T) {
	h, rt := newTestSetup(t)
	nsKey := testNSKey()

	key := startProcess(t, rt, "sleep 10")

	sig := domainstep.NewSignalStep("stop", "SIGTERM", 5*time.Second, true)
	err := h.Execute(context.Background(), wizstep.Request{NSKey: nsKey, Tracker: trackerWithKey(key)}, sig)

	assert.NoError(t, err)
}

func TestHandler_Execute_NoProcess(t *testing.T) {
	h, _ := newTestSetup(t)

	sig := domainstep.NewSignalStep("stop", "SIGTERM", 5*time.Second, true)
	err := h.Execute(context.Background(), wizstep.Request{NSKey: testNSKey()}, sig)

	assert.ErrorIs(t, err, ErrNoProcess)
}

func TestHandler_Execute_NilTracker(t *testing.T) {
	h, _ := newTestSetup(t)

	sig := domainstep.NewSignalStep("stop", "SIGTERM", 5*time.Second, true)
	req := wizstep.Request{NSKey: testNSKey(), Tracker: nil}
	err := h.Execute(context.Background(), req, sig)

	assert.ErrorIs(t, err, ErrNoProcess)
}

func TestHandler_Execute_GhostKey(t *testing.T) {
	h, _ := newTestSetup(t)

	sig := domainstep.NewSignalStep("stop", "SIGTERM", 5*time.Second, true)
	req := wizstep.Request{NSKey: testNSKey(), Tracker: trackerWithKey("ghost-key-not-in-runtime")}
	err := h.Execute(context.Background(), req, sig)

	assert.ErrorIs(t, err, ErrNoProcess)
}

func TestHandler_Execute_InvalidSignal(t *testing.T) {
	h, rt := newTestSetup(t)
	nsKey := testNSKey()

	key := startProcess(t, rt, "sleep 10")

	sig := domainstep.NewSignalStep("bad", "SIGNOTEXIST", 5*time.Second, true)
	err := h.Execute(context.Background(), wizstep.Request{NSKey: nsKey, Tracker: trackerWithKey(key)}, sig)

	assert.ErrorIs(t, err, ErrInvalidSignal)

	kill := domainstep.NewSignalStep("kill", "SIGKILL", 5*time.Second, false)
	_ = h.Execute(context.Background(), wizstep.Request{NSKey: nsKey, Tracker: trackerWithKey(key)}, kill)
}

func TestHandler_Execute_Timeout(t *testing.T) {
	h, rt := newTestSetup(t)
	nsKey := testNSKey()

	key := startProcess(t, rt, "sleep 100")

	// SIGCONT is a no-op on a running process — the process stays alive,
	// guaranteeing we hit the step timeout before Done() fires.
	sig := domainstep.NewSignalStep("cont", "SIGCONT", 100*time.Millisecond, true)
	err := h.Execute(context.Background(), wizstep.Request{NSKey: nsKey, Tracker: trackerWithKey(key)}, sig)

	assert.Error(t, err, "should time out waiting for process that ignores SIGCONT")

	kill := domainstep.NewSignalStep("kill", "SIGKILL", 5*time.Second, false)
	_ = h.Execute(context.Background(), wizstep.Request{NSKey: nsKey, Tracker: trackerWithKey(key)}, kill)
}
