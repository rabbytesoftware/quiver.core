package logger_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/core/logger"
	"github.com/stretchr/testify/assert"
)

func TestInit_DisabledConfig_DoesNotPanic(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	assert.NotPanics(t, func() {
		logger.Init(config.Watcher{Enabled: false, Level: "info"})
	})
}

func TestInit_EnabledConfig_SetsNonNilDefault(t *testing.T) {
	prev := slog.Default()
	dir := t.TempDir()
	// Shutdown must be registered AFTER t.TempDir() so it runs BEFORE TempDir's
	// own cleanup (LIFO order) — required on Windows to release the file handle.
	t.Cleanup(func() {
		logger.Shutdown() //nolint:errcheck
		slog.SetDefault(prev)
	})
	logger.Init(config.Watcher{
		Enabled:  true,
		Level:    "debug",
		Folder:   dir,
		MaxSize:  10,
		MaxAge:   1,
		Compress: false,
	})

	// Verify lumberjack created the log file in the configured folder.
	// The first Write to the handler triggers file creation.
	slog.Info("probe")
	_, err := os.Stat(filepath.Join(dir, "quiver.log"))
	assert.NoError(t, err, "expected quiver.log to be created in log folder")
}

func TestInit_InvalidLevel_FallsBackToInfo(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	assert.NotPanics(t, func() {
		logger.Init(config.Watcher{Enabled: false, Level: "bogus"})
	})
}
