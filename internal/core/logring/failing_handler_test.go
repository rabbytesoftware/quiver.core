package logring_test

import (
	"context"
	"errors"
	"log/slog"
)

var errWrapped = errors.New("wrapped handler failed")

type failingHandler struct{}

func (failingHandler) Enabled(
	_ context.Context,
	_ slog.Level,
) bool {
	return true
}

func (failingHandler) Handle(
	_ context.Context,
	_ slog.Record,
) error {
	return errWrapped
}

func (h failingHandler) WithAttrs(
	_ []slog.Attr,
) slog.Handler {
	return h
}

func (h failingHandler) WithGroup(
	_ string,
) slog.Handler {
	return h
}
