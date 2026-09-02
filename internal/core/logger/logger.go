package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
)

const (
	logFilename  = "Quiver.log"
	logMaxSizeMB = 5
)

// Init configures slog.Default for the lifetime of the process.
// When cfg.Enabled is false, logs go to stderr only.
// When cfg.Enabled is true, logs go to both stdout and a rotating file
// under the Quiver logs directory (~/.quiver/logs/Quiver.log).
// Returns a shutdown function that closes the log file; call it before process exit.
func Init(cfg config.Logger) func() error {
	return initAt("", cfg)
}

// InitAt is Init rooted at homeDir instead of the process-level HOME — used
// when the caller was built with an explicit home override (tests, or a dev
// build's checkout-local .quiver), so logging never lands in the real
// ~/.quiver.
func InitAt(homeDir string, cfg config.Logger) func() error {
	return initAt(homeDir, cfg)
}

func initAt(homeDir string, cfg config.Logger) func() error {
	roller, handler := buildHandler(homeDir, cfg)
	slog.SetDefault(slog.New(handler))
	return func() error {
		if roller != nil {
			return roller.Close()
		}
		return nil
	}
}

func buildHandler(homeDir string, cfg config.Logger) (*lumberjack.Logger, slog.Handler) {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}

	if !cfg.Enabled {
		return nil, slog.NewTextHandler(os.Stderr, opts)
	}

	var logsPath string
	var err error
	if homeDir != "" {
		logsPath, err = paths.LogsAt(homeDir)
	} else {
		logsPath, err = paths.Logs()
	}
	if err != nil {
		return nil, slog.NewTextHandler(os.Stderr, opts)
	}

	roller := &lumberjack.Logger{
		Filename:  filepath.Join(logsPath, logFilename),
		MaxSize:   logMaxSizeMB,
		Compress:  true,
		LocalTime: true,
	}

	return roller, slog.NewJSONHandler(io.MultiWriter(os.Stdout, roller), opts)
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
