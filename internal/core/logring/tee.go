package logring

import (
	"context"
	"log/slog"
	"time"
)

type teeHandler struct {
	next   slog.Handler
	ring   *ringBuffer
	pre    []preAttr
	prefix string
}

func (h *teeHandler) Enabled(
	ctx context.Context,
	level slog.Level,
) bool {
	return h.next.Enabled(ctx, level)
}

func (h *teeHandler) Handle(
	ctx context.Context,
	r slog.Record,
) error {
	err := h.next.Handle(ctx, r)
	h.ring.add(h.capture(r))
	return err
}

func (h *teeHandler) capture(
	r slog.Record,
) Record {
	col := newCollector()
	for _, p := range h.pre {
		col.addAttr(p.prefix, p.attr)
	}
	r.Attrs(func(attr slog.Attr) bool {
		col.addAttr(h.prefix, attr)
		return true
	})

	at := r.Time
	if at.IsZero() {
		at = time.Now()
	}
	return Record{
		Time:            at.UTC(),
		Level:           LevelName(r.Level),
		Component:       col.component,
		Msg:             truncate(r.Message),
		Fields:          col.fields,
		FieldsTruncated: col.truncated,
	}
}

func (h *teeHandler) WithAttrs(
	attrs []slog.Attr,
) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	pre := make([]preAttr, len(h.pre), len(h.pre)+len(attrs))
	copy(pre, h.pre)
	for _, attr := range attrs {
		pre = append(pre, preAttr{prefix: h.prefix, attr: attr})
	}
	return &teeHandler{
		next:   h.next.WithAttrs(attrs),
		ring:   h.ring,
		pre:    pre,
		prefix: h.prefix,
	}
}

func (h *teeHandler) WithGroup(
	name string,
) slog.Handler {
	if name == "" {
		return h
	}
	return &teeHandler{
		next:   h.next.WithGroup(name),
		ring:   h.ring,
		pre:    h.pre,
		prefix: h.prefix + name + ".",
	}
}
