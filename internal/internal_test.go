package internal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestContainer(t *testing.T) *Container {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	c, err := New(context.Background(), "v0.0.0-test", "test-build")
	require.NoError(t, err)
	return c
}

func TestNew_Success_WiresEveryLayer(t *testing.T) {
	c := newTestContainer(t)
	t.Cleanup(func() { _ = c.Shutdown() })

	assert.NotNil(t, c.Engines)
	assert.NotNil(t, c.Adapters)
	assert.NotNil(t, c.App)
	assert.NotNil(t, c.API)
}

func TestNew_UnusableHome_ReturnsEngineError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	require.NoError(t, os.WriteFile(home, []byte("not a directory"), 0o600))
	t.Setenv("HOME", home)

	_, err := New(context.Background(), "v0.0.0-test", "test-build")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal: engine")
}

func TestRunPhase_GivesTheFuncItsOwnDeadline(t *testing.T) {
	var deadline time.Time
	var hasDeadline bool

	require.NoError(t, runPhase(appDrainTimeout, func(ctx context.Context) error {
		deadline, hasDeadline = ctx.Deadline()
		return nil
	}))

	require.True(t, hasDeadline, "a phase must run under a deadline of its own")
	assert.False(t, deadline.IsZero())
}

func TestRunPhase_ReturnsTheFuncError(t *testing.T) {
	phaseErr := errors.New("phase boom")

	err := runPhase(appDrainTimeout, func(_ context.Context) error {
		return phaseErr
	})

	assert.ErrorIs(t, err, phaseErr)
}

// recordingPhases builds a phase per name, each appending itself to ran when
// invoked and returning the error registered for it.
func recordingPhases(
	ran *[]string,
	errs map[string]error,
	names ...string,
) []shutdownPhase {
	phases := make([]shutdownPhase, 0, len(names))
	for _, name := range names {
		phases = append(phases, shutdownPhase{
			name:    name,
			timeout: appDrainTimeout,
			run: func(_ context.Context) error {
				*ran = append(*ran, name)
				return errs[name]
			},
		})
	}
	return phases
}

func TestRunShutdown_RunsPhasesInOrder(t *testing.T) {
	var ran []string

	require.NoError(t, runShutdown(recordingPhases(&ran, nil, "first", "second", "third")))
	assert.Equal(t, []string{"first", "second", "third"}, ran)
}

func TestRunShutdown_RunsEveryPhaseDespiteFailure(t *testing.T) {
	var ran []string
	firstErr := errors.New("first boom")

	err := runShutdown(recordingPhases(
		&ran,
		map[string]error{"first": firstErr},
		"first", "second", "third",
	))

	require.Error(t, err)
	assert.ErrorIs(t, err, firstErr)
	assert.Equal(t, []string{"first", "second", "third"}, ran,
		"a failed phase must not skip the ones after it")
}

func TestRunShutdown_JoinsEveryPhaseError(t *testing.T) {
	var ran []string
	firstErr := errors.New("first boom")
	lastErr := errors.New("last boom")

	err := runShutdown(recordingPhases(
		&ran,
		map[string]error{"first": firstErr, "last": lastErr},
		"first", "middle", "last",
	))

	require.Error(t, err)
	assert.ErrorIs(t, err, firstErr)
	assert.ErrorIs(t, err, lastErr)
	assert.Contains(t, err.Error(), "internal: first")
	assert.Contains(t, err.Error(), "internal: last")
}

func TestRunShutdown_ExpiredPhaseDoesNotStarveTheNext(t *testing.T) {
	var next error

	err := runShutdown([]shutdownPhase{
		{
			name:    "burns its budget",
			timeout: time.Nanosecond,
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
		},
		{
			name:    "next",
			timeout: appDrainTimeout,
			run: func(ctx context.Context) error {
				next = ctx.Err()
				return nil
			},
		},
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NoError(t, next, "a phase that spent its whole budget must not hand the next one a dead context")
}

func TestContainer_ShutdownPhases_ClosesStoresAfterEveryDrain(t *testing.T) {
	c := newTestContainer(t)
	t.Cleanup(func() { _ = c.Shutdown() })

	names := make([]string, 0, 4)
	for _, p := range c.shutdownPhases() {
		names = append(names, p.name)
	}

	assert.Equal(t,
		[]string{"api shutdown", "app shutdown", "engine shutdown", "adapters close"},
		names,
		"stores must close only after every aggregate has drained")
}

func TestContainer_Shutdown_DrainsAndClosesEveryLayer(t *testing.T) {
	c := newTestContainer(t)

	require.NoError(t, c.Shutdown())
}

func TestContainer_Shutdown_RunsEveryPhaseDespiteFailure(t *testing.T) {
	c := newTestContainer(t)

	require.NoError(t, c.Shutdown())

	// A second pass fails on every already-drained aggregate. Both the app and
	// the engine phase must appear: the engine phase runs only if the app
	// phase's failure did not abort the sequence.
	err := c.Shutdown()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal: app shutdown")
	assert.Contains(t, err.Error(), "internal: engine shutdown")
}

func TestContainer_Start_ShutdownFails_ReturnsError(t *testing.T) {
	c := newTestContainer(t)
	require.NoError(t, c.Shutdown())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Start(ctx, "tcp://127.0.0.1:0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal: app shutdown")
}

func TestContainer_Start_ContextCancelled_ShutsDownCleanly(t *testing.T) {
	c := newTestContainer(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, c.Start(ctx, "tcp://127.0.0.1:0"))
}

func TestContainer_Start_ListenerFails_ReturnsGatewayError(t *testing.T) {
	c := newTestContainer(t)
	t.Cleanup(func() { _ = c.Shutdown() })

	err := c.Start(context.Background(), "grpc://127.0.0.1:0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal: gateway")
}
