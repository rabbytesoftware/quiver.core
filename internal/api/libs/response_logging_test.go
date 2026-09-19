package libs_test

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
)

// captureLogs swaps the default slog handler for the duration of the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	return &buf
}

func newLoggingCtx() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	return c, w
}

// A 500 means the error could not be classified, so its chain is about to be
// replaced by a constant and dropped. The log is the only record left of it.
func TestWriteErr_LogsTheCauseOnServerError(t *testing.T) {
	buf := captureLogs(t)
	c, w := newLoggingCtx()

	cause := errors.New("add: reader resolve for install: something specific")
	libs.WriteErr(c, http.StatusInternalServerError, "internal error", "github.com/u/r", cause)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, buf.String(), "something specific",
		"the discarded chain must survive in the log")
	assert.Contains(t, buf.String(), "github.com/u/r")
}

// A classified 4xx already tells the caller what went wrong, so logging it
// would be noise on every bad request.
func TestWriteErr_DoesNotLogClientErrors(t *testing.T) {
	buf := captureLogs(t)
	c, _ := newLoggingCtx()

	libs.WriteErr(c, http.StatusNotFound, "not found", "github.com/u/r", errors.New("no such arrow"))

	assert.Empty(t, strings.TrimSpace(buf.String()))
}

// Call sites that pass no cause must keep working and stay silent.
func TestWriteErr_WithoutCauseIsSilent(t *testing.T) {
	buf := captureLogs(t)
	c, w := newLoggingCtx()

	libs.WriteErr(c, http.StatusInternalServerError, "internal error", "")

	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Empty(t, strings.TrimSpace(buf.String()))
}
