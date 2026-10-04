// Package logring keeps the daemon's most recent log records in memory so the
// console can replay them and follow new ones.
package logring

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	capacity   = 5000
	liveBuffer = 256
	replayLast = 500
	maxValue   = 2048
	maxAttrs   = 32
)

// Record is one captured log record, in the shape the console streams it.
type Record struct {
	Type      string         `json:"type"`
	Seq       uint64         `json:"seq"`
	Time      time.Time      `json:"time"`
	Level     string         `json:"level"`
	Component string         `json:"component"`
	Msg       string         `json:"msg"`
	Fields    map[string]any `json:"fields"`

	level slog.Level
}

// Stream is a gap-free view of the ring. Replay holds the records after since
// (the last 500 when since is zero or newer than Seq, which sets Reset), Seq is
// the newest sequence number, and Live then delivers new records until Close or
// until the reader falls 256 records behind.
type Stream struct {
	Replay []Record
	Seq    uint64
	Reset  bool
	Live   <-chan Record
	Close  func()
}

type subscriber struct {
	ch    chan Record
	level slog.Level
}

// Ring is a fixed-size, concurrency-safe store of log records.
type Ring struct {
	mu      sync.Mutex
	records []Record
	seq     uint64
	subs    map[*subscriber]struct{}
	redact  *regexp.Regexp
}

// New returns an empty Ring.
func New() *Ring {
	return &Ring{
		records: make([]Record, capacity),
		subs:    make(map[*subscriber]struct{}),
		redact:  regexp.MustCompile(`(?i)token|secret|passw(or)?d|authorization|api[-_]?key|private[-_]?key|bearer|cookie|credential|pairing`),
	}
}

// Wrap returns a handler that records what next handles, then passes it on.
func (r *Ring) Wrap(next slog.Handler) slog.Handler {
	return &handler{next: next, ring: r}
}

// Stream opens a Stream of the records at or above level.
func (r *Ring) Stream(since uint64, level slog.Level) *Stream {
	r.mu.Lock()
	defer r.mu.Unlock()

	reset := since > r.seq
	if reset {
		since = 0
	}
	limit := uint64(capacity)
	if since == 0 {
		limit = replayLast
	}
	var replay []Record
	for seq := max(since+1, r.seq-min(r.seq, limit)+1); seq <= r.seq; seq++ {
		if rec := r.records[seq%capacity]; rec.level >= level {
			replay = append(replay, rec)
		}
	}

	sub := &subscriber{ch: make(chan Record, liveBuffer), level: level}
	r.subs[sub] = struct{}{}
	return &Stream{Replay: replay, Seq: r.seq, Reset: reset, Live: sub.ch, Close: func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.drop(sub)
	}}
}

func (r *Ring) append(rec Record) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	rec.Seq = r.seq
	r.records[rec.Seq%capacity] = rec
	for sub := range r.subs {
		if rec.level < sub.level {
			continue
		}
		select {
		case sub.ch <- rec:
		default:
			r.drop(sub)
		}
	}
}

func (r *Ring) drop(sub *subscriber) {
	if _, ok := r.subs[sub]; ok {
		delete(r.subs, sub)
		close(sub.ch)
	}
}

type handler struct {
	next   slog.Handler
	ring   *Ring
	prefix string
	attrs  []slog.Attr
}

func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *handler) Handle(ctx context.Context, rec slog.Record) error {
	fields := make(map[string]any)
	for _, a := range h.attrs {
		h.ring.flatten(fields, "", a)
	}
	rec.Attrs(func(a slog.Attr) bool {
		h.ring.flatten(fields, h.prefix, a)
		return true
	})
	component, _ := fields["component"].(string)
	delete(fields, "component")

	h.ring.append(Record{
		Type: "log", Time: rec.Time, Level: strings.ToLower(rec.Level.String()), Component: component,
		Msg: rec.Message, Fields: fields, level: rec.Level,
	})
	return h.next.Handle(ctx, rec)
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.next = h.next.WithAttrs(attrs)
	next.attrs = append([]slog.Attr(nil), h.attrs...)
	for _, a := range attrs {
		a.Key = h.prefix + a.Key
		next.attrs = append(next.attrs, a)
	}
	return &next
}

func (h *handler) WithGroup(name string) slog.Handler {
	next := *h
	next.next = h.next.WithGroup(name)
	if name != "" {
		next.prefix += name + "."
	}
	return &next
}

// flatten stores a under its dotted key, redacting sensitive keys and capping
// string values and the attribute count.
func (r *Ring) flatten(fields map[string]any, prefix string, a slog.Attr) {
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		if a.Key != "" {
			prefix += a.Key + "."
		}
		for _, member := range v.Group() {
			r.flatten(fields, prefix, member)
		}
		return
	}
	key := prefix + a.Key
	switch kind := v.Kind(); {
	case a.Key == "" || len(fields) >= maxAttrs:
	case r.redact.MatchString(key):
		fields[key] = "[redacted]"
	case kind == slog.KindInt64 || kind == slog.KindUint64 || kind == slog.KindFloat64 || kind == slog.KindBool:
		fields[key] = v.Any()
	default:
		fields[key] = v.String()[:min(len(v.String()), maxValue)]
	}
}
