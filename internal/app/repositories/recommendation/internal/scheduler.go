package recommendationinternal

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Scheduler runs refreshes one at a time.
type Scheduler interface {
	// Start launches the loop and returns at once. It refreshes immediately when
	// the snapshot is due, then every interval, until ctx is cancelled.
	Start(
		ctx context.Context,
	)

	// Trigger starts a refresh in the background unless one is already running,
	// in which case it joins that one. The refresh does not depend on ctx being
	// alive once Trigger returns.
	Trigger(
		ctx context.Context,
	)

	// Running reports whether a refresh is in flight.
	Running() bool

	// Shutdown stops the loop, cancels a refresh in flight and waits for both,
	// until ctx expires. Nothing starts afterwards.
	Shutdown(
		ctx context.Context,
	) error
}

// SchedulerConfig is what a Scheduler is built from. Ticker is for tests; nil
// uses a real ticker.
type SchedulerConfig struct {
	Run      func(ctx context.Context) error
	Due      func(ctx context.Context) bool
	Interval time.Duration
	Ticker   func(interval time.Duration) (<-chan time.Time, func())
}

type scheduler struct {
	cfg SchedulerConfig

	mu      sync.Mutex
	wg      sync.WaitGroup
	quit    chan struct{}
	running bool
	closed  bool
	cancel  context.CancelFunc
}

// NewScheduler builds a Scheduler.
func NewScheduler(
	cfg SchedulerConfig,
) Scheduler {
	return &scheduler{cfg: cfg, quit: make(chan struct{})}
}

func (s *scheduler) Start(
	ctx context.Context,
) {
	if !s.track() {
		return
	}
	go s.loop(ctx)
}

func (s *scheduler) track() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.wg.Add(1)
	return true
}

func (s *scheduler) loop(
	ctx context.Context,
) {
	defer s.wg.Done()

	ticks, stop := s.ticker()
	defer stop()

	if s.cfg.Due(ctx) {
		s.launch(ctx)
	}
	for s.wait(ctx, ticks) {
		s.launch(ctx)
	}
}

func (s *scheduler) ticker() (<-chan time.Time, func()) {
	if s.cfg.Ticker != nil {
		return s.cfg.Ticker(s.cfg.Interval)
	}
	ticker := time.NewTicker(s.cfg.Interval)
	return ticker.C, ticker.Stop
}

func (s *scheduler) wait(
	ctx context.Context,
	ticks <-chan time.Time,
) bool {
	select {
	case <-ctx.Done():
		return false
	case <-s.quit:
		return false
	case <-ticks:
		return true
	}
}

func (s *scheduler) Trigger(
	ctx context.Context,
) {
	s.launch(context.WithoutCancel(ctx))
}

func (s *scheduler) launch(
	parent context.Context,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.running {
		return
	}

	ctx, cancel := context.WithCancel(parent)
	s.running = true
	s.cancel = cancel
	s.wg.Add(1)
	go s.run(ctx, cancel)
}

func (s *scheduler) run(
	ctx context.Context,
	cancel context.CancelFunc,
) {
	defer s.wg.Done()
	defer cancel()

	err := s.cfg.Run(ctx)

	s.mu.Lock()
	s.running = false
	s.cancel = nil
	s.mu.Unlock()

	if err != nil {
		slog.ErrorContext(ctx, "recommendation: refresh failed", "err", err)
	}
}

func (s *scheduler) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *scheduler) Shutdown(
	ctx context.Context,
) error {
	s.stop()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *scheduler) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.quit)
	}
	if s.cancel != nil {
		s.cancel()
	}
}
