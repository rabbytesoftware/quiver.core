package selfupdate_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
)

func TestTrigger_NotFiredByDefault(t *testing.T) {
	trig := selfupdate.NewTrigger(nil)

	assert.False(t, trig.Fired())
	assert.Empty(t, trig.NewBinaryPath())
}

func TestTrigger_FireRecordsPath(t *testing.T) {
	trig := selfupdate.NewTrigger(nil)

	trig.Fire("/home/user/.quiver/vault/quiver-core/stable-26.0.0/quiver-new")

	assert.True(t, trig.Fired())
	assert.Equal(t, "/home/user/.quiver/vault/quiver-core/stable-26.0.0/quiver-new", trig.NewBinaryPath())
}

// A second update finishing mid-shutdown must not change the relaunch target:
// the process is already on its way out towards the first one.
func TestTrigger_FireIsIdempotent(t *testing.T) {
	var stops atomic.Int32
	trig := selfupdate.NewTrigger(func() { stops.Add(1) })

	trig.Fire("/path/one")
	trig.Fire("/path/two")

	assert.Equal(t, "/path/one", trig.NewBinaryPath())
	assert.Equal(t, int32(1), stops.Load(), "shutdown must be requested once, not once per fire")
}

// Two updates ending at the same instant land on two different asynx workers,
// so Fire races itself for real. Exactly one path must win, and the recorded
// path must be the one whose fire also requested the shutdown.
func TestTrigger_FireIsIdempotentUnderConcurrency(t *testing.T) {
	var (
		mu     sync.Mutex
		stops  int
		trig   *selfupdate.Trigger
		wg     sync.WaitGroup
		start  = make(chan struct{})
		winner string
	)

	trig = selfupdate.NewTrigger(func() {
		mu.Lock()
		defer mu.Unlock()
		stops++
		winner = trig.NewBinaryPath()
	})

	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			trig.Fire(paths[i%len(paths)])
		}()
	}
	close(start)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, stops)
	assert.Equal(t, winner, trig.NewBinaryPath())
	assert.Contains(t, paths, trig.NewBinaryPath())
}

var paths = []string{"/path/a", "/path/b", "/path/c", "/path/d"}

func TestTrigger_FireWithoutShutdownFunc_DoesNotPanic(t *testing.T) {
	trig := selfupdate.NewTrigger(nil)

	assert.NotPanics(t, func() { trig.Fire("/path/one") })
	assert.True(t, trig.Fired())
}

func TestTrigger_Relaunch_NeverFired_ReturnsError(t *testing.T) {
	trig := selfupdate.NewTrigger(nil)

	err := trig.Relaunch()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no binary path recorded")
}

// Fire("") is reachable only through a bug upstream, but it must fail loudly
// rather than hand an empty path to the OS.
func TestTrigger_Relaunch_FiredWithEmptyPath_ReturnsError(t *testing.T) {
	trig := selfupdate.NewTrigger(nil)
	trig.Fire("")

	require.Error(t, trig.Relaunch())
}
