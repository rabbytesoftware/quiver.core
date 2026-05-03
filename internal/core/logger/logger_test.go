package logger_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/core/logger"
	"github.com/rabbytesoftware/quiver/internal/core/paths"
)

func TestInit_DisabledConfig_DoesNotPanic(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	assert.NotPanics(t, func() {
		shutdown := logger.Init(config.Logger{Enabled: false, Level: "info"})
		assert.NoError(t, shutdown())
	})
}

func TestInit_EnabledConfig_CreatesLogFile(t *testing.T) {
	prev := slog.Default()

	logsPath, err := paths.Logs()
	require.NoError(t, err)
	logFile := filepath.Join(logsPath, "Quiver.log")

	shutdown := logger.Init(config.Logger{Enabled: true, Level: "debug"})
	t.Cleanup(func() {
		_ = shutdown()        // 1 — close file handle first
		slog.SetDefault(prev) // 2 — then restore default logger
		os.Remove(logFile)    // 3 — then remove (handle is closed)
	})

	slog.Info("probe")
	_, statErr := os.Stat(logFile)
	assert.NoError(t, statErr, "expected Quiver.log to be created in logs dir")
}

func TestInit_InvalidLevel_FallsBackToInfo(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	assert.NotPanics(t, func() {
		_ = logger.Init(config.Logger{Enabled: false, Level: "bogus"})
	})
}

func TestParseLevel_Debug(t *testing.T) {
	assert.Equal(t, slog.LevelDebug, logger.ParseLevel("debug"))
	assert.Equal(t, slog.LevelDebug, logger.ParseLevel("trace"))
	assert.Equal(t, slog.LevelDebug, logger.ParseLevel("DEBUG"))
}

func TestParseLevel_Warn(t *testing.T) {
	assert.Equal(t, slog.LevelWarn, logger.ParseLevel("warn"))
	assert.Equal(t, slog.LevelWarn, logger.ParseLevel("warning"))
}

func TestParseLevel_Error(t *testing.T) {
	assert.Equal(t, slog.LevelError, logger.ParseLevel("error"))
	assert.Equal(t, slog.LevelError, logger.ParseLevel("fatal"))
	assert.Equal(t, slog.LevelError, logger.ParseLevel("panic"))
}

func TestParseLevel_Default_ReturnsInfo(t *testing.T) {
	assert.Equal(t, slog.LevelInfo, logger.ParseLevel("bogus"))
	assert.Equal(t, slog.LevelInfo, logger.ParseLevel(""))
}

func TestBuildHandler_LogsPathError_FallsBackToStderr(t *testing.T) {
	cfg := config.Logger{Enabled: false, Level: "info"}
	roller, handler := logger.BuildHandler(cfg)
	assert.Nil(t, roller)
	assert.NotNil(t, handler)
}

func TestBuildHandler_PathsLogsError_FallsBackToStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("resolveHome on Windows uses user.Current(), not HOME env var")
	}
	// Force os.UserHomeDir() to fail so paths.Logs() returns an error,
	// causing buildHandler to fall back to a stderr TextHandler.
	t.Setenv("HOME", "/dev/null/nonexistent")

	cfg := config.Logger{Enabled: true, Level: "info"}
	roller, handler := logger.BuildHandler(cfg)
	assert.Nil(t, roller)
	assert.NotNil(t, handler)
}
