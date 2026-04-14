package logger_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/core/logger"
	"github.com/rabbytesoftware/quiver/internal/core/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_DisabledConfig_DoesNotPanic(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	assert.NotPanics(t, func() {
		_ = logger.Init(config.Logger{Enabled: false, Level: "info"})
	})
}

func TestInit_EnabledConfig_CreatesLogFile(t *testing.T) {
	prev := slog.Default()

	logsPath, err := paths.Logs()
	require.NoError(t, err)
	logFile := filepath.Join(logsPath, "Quiver.log")

	shutdown := logger.Init(config.Logger{Enabled: true, Level: "debug"})
	t.Cleanup(func() {
		slog.SetDefault(prev)
		_ = shutdown()
		os.Remove(logFile)
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
