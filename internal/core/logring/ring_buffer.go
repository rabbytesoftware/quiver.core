package logring

import (
	"log/slog"
	"sync"
)

type ringBuffer struct {
	mu      sync.Mutex
	entries []Record
	next    int
	count   int
	seq     uint64
	subs    map[*queueSubscription]struct{}
}

func (r *ringBuffer) add(
	rec Record,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	rec.Seq = r.seq
	r.entries[r.next] = rec
	r.next = (r.next + 1) % len(r.entries)
	if r.count < len(r.entries) {
		r.count++
	}
	r.deliver(rec)
}

func (r *ringBuffer) deliver(
	rec Record,
) {
	level := ParseLevel(rec.Level)
	for sub := range r.subs {
		sub.offer(rec, level)
	}
}

func (r *ringBuffer) Snapshot(
	since uint64,
	min slog.Level,
	limit int,
) []Record {
	if limit < 1 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	matched := r.matching(since, min)
	if len(matched) > limit {
		matched = matched[len(matched)-limit:]
	}
	return matched
}

func (r *ringBuffer) matching(
	since uint64,
	min slog.Level,
) []Record {
	out := make([]Record, 0, r.count)
	start := (r.next - r.count + len(r.entries)) % len(r.entries)
	for i := 0; i < r.count; i++ {
		out = appendMatch(out, r.entries[(start+i)%len(r.entries)], since, min)
	}
	return out
}

func appendMatch(
	out []Record,
	rec Record,
	since uint64,
	min slog.Level,
) []Record {
	if rec.Seq <= since || ParseLevel(rec.Level) < min {
		return out
	}
	return append(out, rec)
}

func (r *ringBuffer) Latest() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.seq
}

func (r *ringBuffer) Subscribe(
	min slog.Level,
) Subscription {
	sub := newQueueSubscription(r, min)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.subs[sub] = struct{}{}
	return sub
}

func (r *ringBuffer) unsubscribe(
	sub *queueSubscription,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.subs, sub)
}

func (r *ringBuffer) Tee(
	next slog.Handler,
) slog.Handler {
	return &teeHandler{next: next, ring: r}
}
