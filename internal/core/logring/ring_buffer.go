package logring

import (
	"log/slog"
	"regexp"
	"sync"
)

type ringBuffer struct {
	mu          sync.Mutex
	records     []Record
	seq         uint64
	subscribers map[*subscriber]struct{}
	redact      *regexp.Regexp
}

// Wrap returns a slog.Handler that stores each record next handles and then
// passes it to next.
func (r *ringBuffer) Wrap(
	next slog.Handler,
) slog.Handler {
	return &handler{next: next, ring: r}
}

// Stream opens a gap-free Stream of the records at or above level, replaying
// after since or the last 500 records.
func (r *ringBuffer) Stream(
	since uint64,
	level slog.Level,
) Stream {
	r.mu.Lock()
	defer r.mu.Unlock()

	reset := since > r.seq
	if reset {
		since = 0
	}
	sub := newSubscriber(level)
	r.subscribers[sub] = struct{}{}
	return &openStream{ring: r, subscriber: sub, replay: r.replay(since, level), seq: r.seq, reset: reset}
}

func (r *ringBuffer) replay(
	since uint64,
	level slog.Level,
) []Record {
	limit := uint64(capacity)
	if since == 0 {
		limit = replayLast
	}
	var replay []Record
	for seq := max(since+1, r.seq-min(r.seq, limit)+1); seq <= r.seq; seq++ {
		replay = r.appendAtLevel(replay, seq, level)
	}
	return replay
}

func (r *ringBuffer) appendAtLevel(
	replay []Record,
	seq uint64,
	level slog.Level,
) []Record {
	rec := r.records[seq%capacity]
	if rec.level < level {
		return replay
	}
	return append(replay, rec)
}

func (r *ringBuffer) append(
	rec Record,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	rec.Seq = r.seq
	r.records[rec.Seq%capacity] = rec
	for sub := range r.subscribers {
		r.deliver(sub, rec)
	}
}

func (r *ringBuffer) deliver(
	sub *subscriber,
	rec Record,
) {
	if !sub.wants(rec) {
		return
	}
	if !sub.offer(rec) {
		r.remove(sub)
	}
}

func (r *ringBuffer) unsubscribe(
	sub *subscriber,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.remove(sub)
}

func (r *ringBuffer) remove(
	sub *subscriber,
) {
	if _, subscribed := r.subscribers[sub]; !subscribed {
		return
	}
	delete(r.subscribers, sub)
	close(sub.records)
}
