//go:build darwin || linux

package signal

import (
	"context"
	"testing"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/runtime"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/step"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSetup(t *testing.T) (*handler, runtime.Runtime) {
	t.Helper()
	rt, err := runtime.New()
	require.NoError(t, err)
	return NewHandler(rt).(*handler), rt
}

func testNSKey() string {
	return "test/user/repo/arrow"
}

func startProcess(t *testing.T, rt runtime.Runtime, command string) runtime.Process {
	t.Helper()
	config := runtime.NewConfig([]string{command})
	config.ShellWrap = true
	proc, err := rt.Start(context.Background(), config)
	require.NoError(t, err)
	return proc
}

func TestHandler_Execute_Success(t *testing.T) {
	h, rt := newTestSetup(t)

	proc := startProcess(t, rt, "sleep 10")
	defer proc.Kill(context.Background())

	sig := domainstep.NewSignalStep("stop", domainstep.SignalKindGraceful, "5s", true)
	err := h.Execute(context.Background(), wizstep.Request{NSKey: testNSKey(), PID: proc.PID()}, sig)

	assert.NoError(t, err)
}

func TestHandler_Execute_NoProcess(t *testing.T) {
	h, _ := newTestSetup(t)

	sig := domainstep.NewSignalStep("stop", domainstep.SignalKindGraceful, "5s", true)
	err := h.Execute(context.Background(), wizstep.Request{NSKey: testNSKey()}, sig)

	assert.ErrorIs(t, err, ErrNoProcess)
}

func TestHandler_Execute_InvalidSignal(t *testing.T) {
	h, rt := newTestSetup(t)

	proc := startProcess(t, rt, "sleep 10")
	defer proc.Kill(context.Background())

	sig := domainstep.NewSignalStep("bad", domainstep.SignalKind("invalid"), "5s", true)
	err := h.Execute(context.Background(), wizstep.Request{NSKey: testNSKey(), PID: proc.PID()}, sig)

	assert.ErrorIs(t, err, ErrInvalidSignal)
}

func TestHandler_Execute_InvalidTimeout_ReturnsError(t *testing.T) {
	h, rt := newTestSetup(t)

	proc := startProcess(t, rt, "sleep 10")
	defer proc.Kill(context.Background())

	sig := domainstep.NewSignalStep("stop", domainstep.SignalKindGraceful, "not-a-duration", true)
	err := h.Execute(context.Background(), wizstep.Request{NSKey: testNSKey(), PID: proc.PID()}, sig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timeout")
}
