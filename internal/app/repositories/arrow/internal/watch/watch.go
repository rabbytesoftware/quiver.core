package watch

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

// Watch runs the passive version check of every installed row soon after it
// starts and then every interval, so a row reads outdated without anyone
// opening it. A zero interval runs nothing.
type Watch interface {
	Start(ctx context.Context)
	// Stop ends the watch and waits for a sweep in progress, so no check
	// writes to a store the shutdown is about to close.
	Stop()
}

type watch struct {
	interval time.Duration
	sweep    func(ctx context.Context)
	jitter   func(limit time.Duration) time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
}

func New(
	interval time.Duration,
	sweep func(ctx context.Context),
) Watch {
	return &watch{interval: interval, sweep: sweep, jitter: randomJitter}
}

func (w *watch) Start(ctx context.Context) {
	if w.interval <= 0 || w.cancel != nil {
		return
	}
	ctx, w.cancel = context.WithCancel(ctx)
	w.done = make(chan struct{})
	go w.run(ctx)
}

func (w *watch) run(ctx context.Context) {
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

func (w *watch) Stop() {
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

// Interval is override when one was given, and arrows.version_check_interval
// otherwise.
func Interval(
	override *time.Duration,
) time.Duration {
	if override != nil {
		return *override
	}
	return resolveVersionCheckInterval()
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
