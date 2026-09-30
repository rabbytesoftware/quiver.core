package app

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
)

const (
	defaultVersionCheckInterval = 6 * time.Hour
	// firstCheckSpread spreads the first sweep after a boot, so daemons
	// started together do not all ask their remotes at the same moment.
	firstCheckSpread = time.Minute
	// intervalSpread is the fraction of the interval each later sweep is
	// pushed back by, at most.
	intervalSpread = 10
)

// versionWatch runs the passive version check of every installed row soon
// after the daemon starts and then every interval, so a row reads outdated
// without anyone opening it. A zero interval runs nothing.
type versionWatch struct {
	interval time.Duration
	sweep    func(ctx context.Context)
	jitter   func(limit time.Duration) time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
}

func newVersionWatch(
	interval time.Duration,
	sweep func(ctx context.Context),
) *versionWatch {
	return &versionWatch{interval: interval, sweep: sweep, jitter: randomJitter}
}

func (w *versionWatch) start(ctx context.Context) {
	if w.interval <= 0 || w.cancel != nil {
		return
	}
	ctx, w.cancel = context.WithCancel(ctx)
	w.done = make(chan struct{})
	go w.run(ctx)
}

func (w *versionWatch) run(ctx context.Context) {
	defer close(w.done)
	wait := w.jitter(firstCheckSpread)
	for {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		w.sweep(ctx)
		wait = w.interval + w.jitter(w.interval/intervalSpread)
	}
}

// stop ends the watch and waits for a sweep in progress, so no check writes
// to a store the shutdown is about to close.
func (w *versionWatch) stop() {
	if w.cancel == nil {
		return
	}
	w.cancel()
	<-w.done
}

func randomJitter(limit time.Duration) time.Duration {
	if limit <= 0 {
		return 0
	}
	return rand.N(limit) // #nosec G404 -- spreading timers needs no cryptographic randomness
}

// resolveVersionCheckInterval reads arrows.version_check_interval; "0s"
// turns the periodic check off, and an unreadable value falls back to the
// default.
func resolveVersionCheckInterval() time.Duration {
	return parseCheckInterval(config.GetArrows().VersionCheckInterval)
}

func parseCheckInterval(raw string) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return defaultVersionCheckInterval
	}
	return d
}
