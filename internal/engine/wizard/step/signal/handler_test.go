//go:build !windows

package signal

import (
	"context"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/process"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

// testTracker is a minimal ProcessTracker for use in tests.
type testTracker struct {
	mu   sync.RWMutex
	proc process.Process
}

func (t *testTracker) SetProcess(proc process.Process) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.proc = proc
}

func (t *testTracker) GetProcess() (process.Process, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.proc == nil {
		return nil, false
	}
	return t.proc, true
}

func trackerWithProcess(proc process.Process) wizstep.ProcessTracker {
	tr := &testTracker{}
	tr.SetProcess(proc)
	return tr
}

func newTestSetup(t *testing.T) (*handler, *runtime.Runtime) {
	t.Helper()
	rt, err := runtime.New()
	require.NoError(t, err)
	return NewHandler().(*handler), rt
}

func testNSKey() string {
	return domain.Namespace("test/user/repo/arrow").String()
}

func startProcess(t *testing.T, rt *runtime.Runtime, command string) process.Process {
	t.Helper()
	proc, err := rt.Get(context.Background(), command).WithShellWrap().Build()
	require.NoError(t, err)
	require.NoError(t, proc.Start(context.Background()))
	return proc
}

func TestHandler_Execute_Success(t *testing.T) {
	h, rt := newTestSetup(t)

	proc := startProcess(t, rt, "sleep 10")

	sig := domainstep.NewSignalStep("stop", domainstep.SignalKindGraceful, "5s", true)
	err := h.Execute(context.Background(), wizstep.Request{NSKey: testNSKey(), Tracker: trackerWithProcess(proc)}, sig)

	assert.NoError(t, err)
}

func TestHandler_Execute_NoProcess(t *testing.T) {
	h, _ := newTestSetup(t)

	sig := domainstep.NewSignalStep("stop", domainstep.SignalKindGraceful, "5s", true)
	err := h.Execute(context.Background(), wizstep.Request{NSKey: testNSKey()}, sig)

	assert.ErrorIs(t, err, ErrNoProcess)
}

func TestHandler_Execute_NilTracker(t *testing.T) {
	h, _ := newTestSetup(t)

	sig := domainstep.NewSignalStep("stop", domainstep.SignalKindGraceful, "5s", true)
	req := wizstep.Request{NSKey: testNSKey(), Tracker: nil}
	err := h.Execute(context.Background(), req, sig)

	assert.ErrorIs(t, err, ErrNoProcess)
}

func TestHandler_Execute_InvalidSignal(t *testing.T) {
	h, rt := newTestSetup(t)

	proc := startProcess(t, rt, "sleep 10")

	sig := domainstep.NewSignalStep("bad", domainstep.SignalKind("invalid"), "5s", true)
	err := h.Execute(context.Background(), wizstep.Request{NSKey: testNSKey(), Tracker: trackerWithProcess(proc)}, sig)

	assert.ErrorIs(t, err, ErrInvalidSignal)

	kill := domainstep.NewSignalStep("kill", domainstep.SignalKindKill, "5s", false)
	_ = h.Execute(context.Background(), wizstep.Request{NSKey: testNSKey(), Tracker: trackerWithProcess(proc)}, kill)
}

func TestHandler_Execute_Timeout(t *testing.T) {
	h, rt := newTestSetup(t)

	proc := startProcess(t, rt, "sleep 100")

	// SIGCONT is a no-op on a running process — it stays alive, so the step
	// timeout fires before proc.Done().
	sig := domainstep.NewSignalStep("cont", domainstep.SignalKind("SIGCONT"), "100ms", true)
	err := h.Execute(context.Background(), wizstep.Request{NSKey: testNSKey(), Tracker: trackerWithProcess(proc)}, sig)

	assert.Error(t, err, "should time out waiting for process that ignores SIGCONT")

	kill := domainstep.NewSignalStep("kill", domainstep.SignalKindKill, "5s", false)
	_ = h.Execute(context.Background(), wizstep.Request{NSKey: testNSKey(), Tracker: trackerWithProcess(proc)}, kill)
}
