//go:build integration

package integration_test

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	// Suppress app-level slog output (HTTP access logs, engine logs, etc.)
	// and Gin debug mode warnings — neither is useful in CI test output.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}
