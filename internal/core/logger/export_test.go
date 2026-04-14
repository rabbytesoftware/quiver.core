package logger

import "log/slog"

// ParseLevel exposes the unexported parseLevel function for testing.
var ParseLevel = func(level string) slog.Level {
	return parseLevel(level)
}

// BuildHandler exposes the unexported buildHandler for testing.
var BuildHandler = buildHandler
