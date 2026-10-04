package logring

import (
	"log/slog"
	"strings"
)

// ParseLevel maps a level name to its slog level. Unknown names map to info.
func ParseLevel(
	name string,
) slog.Level {
	switch strings.ToLower(name) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// LevelName returns the lower-case wire name of a slog level.
func LevelName(
	level slog.Level,
) string {
	switch {
	case level >= slog.LevelError:
		return "error"
	case level >= slog.LevelWarn:
		return "warn"
	case level >= slog.LevelInfo:
		return "info"
	default:
		return "debug"
	}
}

// IsLevel reports whether name is one of the four wire level names.
func IsLevel(
	name string,
) bool {
	switch name {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}
