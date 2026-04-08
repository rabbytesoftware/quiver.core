package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver/internal/core/config"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// Init configures slog.Default for the lifetime of the process.
// When cfg.Enabled is false, logs go to stderr only.
// When cfg.Enabled is true, logs go to both stdout and a rotating file under cfg.Folder.
// Call once from core.Init() before any other service is started.
func Init(cfg config.Watcher) {
	slog.SetDefault(slog.New(buildHandler(cfg)))
}

func buildHandler(cfg config.Watcher) slog.Handler {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}

	if !cfg.Enabled {
		return slog.NewTextHandler(os.Stderr, opts)
	}

	roller := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.Folder, "quiver.log"),
		MaxSize:    cfg.MaxSize,
		MaxAge:     cfg.MaxAge,
		MaxBackups: 3,
		Compress:   cfg.Compress,
		LocalTime:  true,
	}

	return slog.NewJSONHandler(io.MultiWriter(os.Stdout, roller), opts)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug", "trace":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "fatal", "panic":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
