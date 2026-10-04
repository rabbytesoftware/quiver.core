package logring

import (
	"fmt"
	"log/slog"
	"math"
	"time"
	"unicode/utf8"
)

const (
	componentKey = "component"
	maxFields    = 32
	maxValue     = 2048
	ellipsis     = "…"
)

type collector struct {
	fields    map[string]any
	component string
	truncated bool
}

func newCollector() *collector {
	return &collector{fields: make(map[string]any)}
}

func (c *collector) addAttr(
	prefix string,
	attr slog.Attr,
) {
	if attr.Equal(slog.Attr{}) {
		return
	}

	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		c.addGroup(prefix, attr.Key, value.Group())
		return
	}

	c.put(prefix+attr.Key, value)
}

func (c *collector) addGroup(
	prefix string,
	key string,
	attrs []slog.Attr,
) {
	if key != "" {
		prefix = prefix + key + "."
	}
	for _, attr := range attrs {
		c.addAttr(prefix, attr)
	}
}

func (c *collector) put(
	key string,
	value slog.Value,
) {
	if key == componentKey {
		c.component = truncate(safeString(value.Any()))
		return
	}
	if len(c.fields) >= maxFields {
		c.truncated = true
		return
	}
	c.fields[key] = RedactValue(key, convert(value))
}

func convert(
	value slog.Value,
) any {
	switch value.Kind() {
	case slog.KindInt64:
		return value.Int64()
	case slog.KindUint64:
		return value.Uint64()
	case slog.KindFloat64:
		return floatValue(value.Float64())
	case slog.KindBool:
		return value.Bool()
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().UTC().Format(time.RFC3339Nano)
	case slog.KindString, slog.KindAny, slog.KindGroup, slog.KindLogValuer:
		return truncate(safeString(value.Any()))
	default:
		return truncate(safeString(value.Any()))
	}
}

func floatValue(
	f float64,
) any {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return fmt.Sprint(f)
	}
	return f
}

func safeString(
	v any,
) (out string) {
	defer recoverString(&out)

	if err, ok := v.(error); ok {
		return err.Error()
	}
	return fmt.Sprint(v)
}

func truncate(
	s string,
) string {
	if len(s) <= maxValue {
		return s
	}

	cut := maxValue
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + ellipsis
}

func recoverString(
	out *string,
) {
	if recover() != nil {
		*out = "[unprintable]"
	}
}
