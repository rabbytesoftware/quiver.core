package logring

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const subscriptionQueue = 256

type queueSubscription struct {
	ring      *ringBuffer
	min       slog.Level
	queue     chan Record
	dropped   atomic.Uint64
	fullSince atomic.Int64
	closeOnce sync.Once
}

func newQueueSubscription(
	ring *ringBuffer,
	min slog.Level,
) *queueSubscription {
	return &queueSubscription{
		ring:  ring,
		min:   min,
		queue: make(chan Record, subscriptionQueue),
	}
}

func (s *queueSubscription) offer(
	rec Record,
	level slog.Level,
) {
	if level < s.min {
		return
	}

	select {
	case s.queue <- rec:
		s.fullSince.Store(0)
	default:
		s.dropped.Add(1)
		s.fullSince.CompareAndSwap(0, time.Now().UnixNano())
	}
}

func (s *queueSubscription) C() <-chan Record {
	return s.queue
}

func (s *queueSubscription) TakeDropped() uint64 {
	return s.dropped.Swap(0)
}

func (s *queueSubscription) StuckFor() time.Duration {
	since := s.fullSince.Load()
	if since == 0 {
		return 0
	}
	return time.Since(time.Unix(0, since))
}

func (s *queueSubscription) Close() {
	s.closeOnce.Do(s.shutdown)
}

func (s *queueSubscription) shutdown() {
	s.ring.unsubscribe(s)
	s.ring.mu.Lock()
	defer s.ring.mu.Unlock()

	close(s.queue)
}
