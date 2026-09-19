// Package selfupdate holds the process-lifetime decision to hand quiver.core
// over to a newer build of itself: whether an update finished, which binary it
// produced, and how this process is replaced by it.
package selfupdate

import (
	"errors"
	"sync"
)

// Trigger records that quiver.core's own update lifecycle finished and that
// this process should hand over to the binary it produced.
//
// It is written once, from the asynx subscriber that sees quiver.core's own
// runtime.ended event, and read once, by cmd/quiver's daemon command after the
// normal shutdown sequence has already run. It is deliberately not a channel:
// the relaunch decision is acted on after Start returns, not concurrently with
// it.
//
// Firing is what starts the shutdown. A trigger that only recorded the
// handover would leave the daemon blocked on its own context until a signal
// arrived, so Fire calls the shutdown function it was built with — making
// "fired" and "shutting down" the same event rather than two the caller has to
// keep in step.
type Trigger struct {
	mu          sync.Mutex
	fired       bool
	binPath     string
	requestStop func()
	relaunch    func(binPath string) error
}

// NewTrigger builds a Trigger that calls requestStop the first time it fires.
// requestStop is the daemon's own cancellation — the same one a SIGTERM would
// reach — so self-succession runs the one graceful shutdown sequence rather
// than a second path beside it. A nil requestStop records the handover without
// asking for one, which is what a test that only inspects state wants.
func NewTrigger(
	requestStop func(),
) *Trigger {
	return &Trigger{
		requestStop: requestStop,
		relaunch:    handOver,
	}
}

// Fire records newBinaryPath as the binary this process hands over to and asks
// the daemon to shut down. Only the first call has any effect: a second update
// finishing while the first is already shutting down must not redirect the
// relaunch, and must not request a second shutdown of a daemon that is already
// leaving.
func (t *Trigger) Fire(
	newBinaryPath string,
) {
	if !t.arm(newBinaryPath) {
		return
	}
	if t.requestStop == nil {
		return
	}
	t.requestStop()
}

// Fired reports whether an update has claimed this process.
func (t *Trigger) Fired() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.fired
}

// NewBinaryPath returns the binary recorded by the first Fire, or the empty
// string if the trigger never fired.
func (t *Trigger) NewBinaryPath() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.binPath
}

// Relaunch hands this process over to the binary recorded by Fire, and must
// only be called once the normal shutdown sequence has already run: the
// listener is closed and every aggregate drained, exactly as for a plain
// SIGTERM. How the handover happens is per-OS — see the exec-replace on unix
// and the detached spawn on windows — but on neither does this function
// perform a stop of its own.
func (t *Trigger) Relaunch() error {
	binPath := t.NewBinaryPath()
	if binPath == "" {
		return errors.New("selfupdate: relaunch: no binary path recorded")
	}

	return t.relaunch(binPath)
}

func (t *Trigger) arm(
	newBinaryPath string,
) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.fired {
		return false
	}
	t.fired = true
	t.binPath = newBinaryPath

	return true
}
