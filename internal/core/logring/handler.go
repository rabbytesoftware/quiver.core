package logring

import (
	"context"
	"log/slog"
	"strings"
	"unicode/utf8"
)

type handler struct {
	next   slog.Handler
	ring   *ringBuffer
	prefix string
	attrs  []slog.Attr
}

// Enabled reports whether the wrapped handler wants records at level.
func (h *handler) Enabled(
	ctx context.Context,
	level slog.Level,
) bool {
	return h.next.Enabled(ctx, level)
}

// Handle stores the record in the ring, then passes it to the wrapped handler
// and returns that handler's error. The ring keeps the record even when the
// wrapped handler fails to write it.
func (h *handler) Handle(
	ctx context.Context,
	rec slog.Record,
) error {
	fields := h.fieldsOf(rec)
	component, _ := fields["component"].(string)
	delete(fields, "component")

	h.ring.append(Record{
		Type:      "log",
		Time:      rec.Time,
		Level:     strings.ToLower(rec.Level.String()),
		Component: component,
		Msg:       truncate(rec.Message),
		Fields:    fields,
		level:     rec.Level,
	})
	return h.next.Handle(ctx, rec)
}

// WithAttrs returns a handler that adds attrs, under the current group, to
// every record it stores, and passes them on to the wrapped handler.
func (h *handler) WithAttrs(
	attrs []slog.Attr,
) slog.Handler {
	derived := *h
	derived.next = h.next.WithAttrs(attrs)
	derived.attrs = append([]slog.Attr(nil), h.attrs...)
	for _, attr := range attrs {
		attr.Key = h.prefix + attr.Key
		derived.attrs = append(derived.attrs, attr)
	}
	return &derived
}

// WithGroup returns a handler that prefixes the keys of every later attribute
// with name and a dot, and opens the group on the wrapped handler.
func (h *handler) WithGroup(
	name string,
) slog.Handler {
	derived := *h
	derived.next = h.next.WithGroup(name)
	if name != "" {
		derived.prefix += name + "."
	}
	return &derived
}

func (h *handler) fieldsOf(
	rec slog.Record,
) map[string]any {
	fields := make(map[string]any)
	for _, attr := range h.attrs {
		h.flatten(fields, "", attr)
	}
	rec.Attrs(func(
		attr slog.Attr,
	) bool {
		h.flatten(fields, h.prefix, attr)
		return true
	})
	return fields
}

func (h *handler) flatten(
	fields map[string]any,
	prefix string,
	attr slog.Attr,
) {
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		h.flattenGroup(fields, prefix, attr.Key, value.Group())
		return
	}
	if attr.Key == "" || len(fields) >= maxAttrs {
		return
	}
	fields[prefix+attr.Key] = h.render(prefix+attr.Key, value)
}

func (h *handler) flattenGroup(
	fields map[string]any,
	prefix string,
	key string,
	members []slog.Attr,
) {
	if key != "" {
		prefix += key + "."
	}
	for _, member := range members {
		h.flatten(fields, prefix, member)
	}
}

func (h *handler) render(
	key string,
	value slog.Value,
) any {
	if h.ring.redact.MatchString(key) {
		return redactedText
	}
	if isScalar(value.Kind()) {
		return value.Any()
	}
	return truncate(value.String())
}

func truncate(
	text string,
) string {
	if len(text) <= maxValue {
		return text
	}
	cut := maxValue
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}

func isScalar(
	kind slog.Kind,
) bool {
	return kind == slog.KindInt64 || kind == slog.KindUint64 || kind == slog.KindFloat64 || kind == slog.KindBool
}
