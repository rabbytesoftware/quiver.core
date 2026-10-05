package console

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func newTestHandlers(
	started chan struct{},
) *Handlers {
	h := New(logring.New(), "v")
	h.tree = (&testTree{started: started}).build
	return h
}

func setDevice(
	c *gin.Context,
) {
	if id := c.GetHeader("X-Device"); id != "" {
		c.Set(middleware.DeviceContextKey, auth.Device{ID: id})
	}
}

func post(
	h *Handlers,
	req *http.Request,
) *httptest.ResponseRecorder {
	r := gin.New()
	r.Use(setDevice)
	r.POST("/exec", h.Exec)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func execRequest(
	body string,
) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/exec", strings.NewReader(body))
}

func lineRequest(
	line string,
) *http.Request {
	body, _ := json.Marshal(map[string]string{"line": line})
	return execRequest(string(body))
}

func readFrames(
	t *testing.T,
	body io.Reader,
) []map[string]any {
	t.Helper()

	var frames []map[string]any
	decoder := json.NewDecoder(body)
	for decoder.More() {
		var frame map[string]any
		require.NoError(t, decoder.Decode(&frame))
		frames = append(frames, frame)
	}
	return frames
}

func captureLogs(
	t *testing.T,
) *strings.Builder {
	t.Helper()

	var logs strings.Builder
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
	return &logs
}

func exitFrame(
	code int,
	message string,
) map[string]any {
	return map[string]any{"type": "exit", "code": float64(code), "error": message}
}

func lastFrame(
	t *testing.T,
	h *Handlers,
	line string,
) map[string]any {
	t.Helper()

	frames := readFrames(t, post(h, lineRequest(line)).Body)
	return frames[len(frames)-1]
}

func TestHandlers_Exec_StreamsOutFramesThenExactlyOneExitFrame(
	t *testing.T,
) {
	logs := captureLogs(t)
	req := lineRequest("echo hunter2")
	req.Header.Set("X-Device", "dev-7")

	rec := post(newTestHandlers(nil), req)

	assert.Equal(t, "application/x-ndjson", rec.Header().Get("Content-Type"))
	assert.Equal(t, []map[string]any{
		{"type": "out", "stream": "stdout", "data": "hello\n"},
		{"type": "out", "stream": "stderr", "data": "careful\n"},
		exitFrame(0, ""),
	}, readFrames(t, rec.Body))
	for _, want := range []string{"component=console", "msg=exec ", "device=device:dev-7", `command="quiver echo"`, "code=0"} {
		assert.Contains(t, logs.String(), want)
	}
	assert.NotContains(t, logs.String(), "hunter2", "the audit record names the resolved command, never the raw line")
}

func TestHandlers_Exec_UnauthenticatedCaller_IsAuditedAsTheUnixCaller(
	t *testing.T,
) {
	logs := captureLogs(t)

	post(newTestHandlers(nil), lineRequest("echo"))

	assert.Contains(t, logs.String(), "device=unix")
}

func TestHandlers_Exec_PairedDeviceNamedUnix_NeverPassesForTheSocketCaller(
	t *testing.T,
) {
	logs := captureLogs(t)
	req := lineRequest("echo")
	req.Header.Set("X-Device", "unix")

	post(newTestHandlers(nil), req)

	assert.Contains(t, logs.String(), "device=device:unix")
}

func TestHandlers_Exec_CommandGivenANamespace_IsAuditedWithIt(
	t *testing.T,
) {
	logs := captureLogs(t)

	post(newTestHandlers(nil), lineRequest("echo github.com/quiver-test/tool@v1"))
	post(newTestHandlers(nil), lineRequest("echo hunter2"))

	assert.Equal(t, 1, strings.Count(logs.String(), "namespace=github.com/quiver-test/tool@v1"))
	assert.Equal(t, 1, strings.Count(logs.String(), `namespace="" `))
}

func assertDenialLogs(
	t *testing.T,
	line string,
	wantCommand string,
) {
	t.Helper()

	logs := captureLogs(t)

	post(newTestHandlers(nil), lineRequest(line))

	assert.Contains(t, logs.String(), "msg=\"exec denied\"")
	assert.Contains(t, logs.String(), "command="+wantCommand, line)
	assert.NotContains(t, logs.String(), "hunter2")
}

func TestHandlers_Exec_Denied_IsAuditedWithTheCommandTheCallerNamed(
	t *testing.T,
) {
	assertDenialLogs(t, "nope now", "nope")
	assertDenialLogs(t, "hidden", `"quiver hidden"`)
	assertDenialLogs(t, "--token hunter2", `""`)
}

func TestHandlers_Exec_FailingCommand_EndsWithItsErrorAndCode(
	t *testing.T,
) {
	assert.Equal(t, exitFrame(1, "boom"), lastFrame(t, newTestHandlers(nil), "fail"))
}

func TestHandlers_Exec_ConfirmationPrompt_ReadsNothingSoItAnswersNo(
	t *testing.T,
) {
	assert.Equal(t, exitFrame(1, "EOF"), lastFrame(t, newTestHandlers(nil), "ask"))
}

func TestHandlers_Exec_PanickingCommand_BecomesAnExitFrameAndAnErrorLog(
	t *testing.T,
) {
	logs := captureLogs(t)

	got := lastFrame(t, newTestHandlers(nil), "panic")

	assert.Equal(t, exitFrame(1, "internal error"), got)
	assert.Contains(t, logs.String(), "level=ERROR")
	assert.Contains(t, logs.String(), "kaboom")
}

func TestHandlers_Exec_OutputPastTheLimit_IsCutWithOneNote(
	t *testing.T,
) {
	frames := readFrames(t, post(newTestHandlers(nil), lineRequest("flood")).Body)

	require.Len(t, frames, 4, "two out frames, one note, one exit: the write after the cut is dropped")
	assert.Equal(t, outputLimit, len(frames[0]["data"].(string))+len(frames[1]["data"].(string)))
	assert.Equal(t, truncationNote, frames[2]["data"])
	assert.Equal(t, exitFrame(0, ""), frames[3])
}

func TestHandlers_Exec_DisconnectedClient_CancelsTheCommand(
	t *testing.T,
) {
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *httptest.ResponseRecorder)
	go func() {
		done <- post(newTestHandlers(started), lineRequest("wait").WithContext(ctx))
	}()

	<-started
	cancel()

	select {
	case rec := <-done:
		assert.Equal(t, exitFrame(1, "context canceled"), readFrames(t, rec.Body)[0])
	case <-time.After(5 * time.Second):
		t.Fatal("the command was not cancelled")
	}
}

func assertRefused(
	t *testing.T,
	req *http.Request,
	busy bool,
	wantStatus int,
	wantMessage string,
) {
	t.Helper()

	logs := captureLogs(t)
	h := newTestHandlers(nil)
	for i := 0; busy && i < maxRunning; i++ {
		h.slots <- struct{}{}
	}

	rec := post(h, req)

	assert.Equal(t, wantStatus, rec.Code, wantMessage)
	assert.Contains(t, rec.Body.String(), wantMessage)
	assert.NotContains(t, rec.Body.String(), `"type":"exit"`)
	assert.Equal(t, wantStatus == http.StatusForbidden, strings.Contains(logs.String(), "level=WARN"), wantMessage)
}

func TestHandlers_Exec_BadRequest_IsRefusedBeforeRunning(
	t *testing.T,
) {
	assertRefused(t, execRequest("nope"), false, http.StatusBadRequest, "request body")
	assertRefused(t, execRequest(`{"line":"`+strings.Repeat("a", 5000)+`"}`), false, http.StatusBadRequest, "request body")
	assertRefused(t, lineRequest(""), false, http.StatusBadRequest, "1 to 1024")
	assertRefused(t, lineRequest("   "), false, http.StatusBadRequest, "1 to 1024")
	assertRefused(t, lineRequest(strings.Repeat("a", 1025)), false, http.StatusBadRequest, "1 to 1024")
	assertRefused(t, lineRequest("echo; fail"), false, http.StatusBadRequest, "quoting is not supported")
	assertRefused(t, lineRequest("echo \"x\""), false, http.StatusBadRequest, "quoting is not supported")
	assertRefused(t, lineRequest("echo\u0085fail"), false, http.StatusBadRequest, "quoting is not supported")
}

func TestHandlers_Exec_CommandNotAllowed_IsForbiddenAndNamed(
	t *testing.T,
) {
	assertRefused(t, lineRequest("hidden"), false, http.StatusForbidden, "hidden")
	assertRefused(t, lineRequest("nope"), false, http.StatusForbidden, "nope")
}

func TestHandlers_Exec_TooManyRunning_IsTooManyRequests(
	t *testing.T,
) {
	assertRefused(t, lineRequest("echo"), true, http.StatusTooManyRequests, "too many")
}

func TestHandlers_Exec_LineOfExactly1024Bytes_IsAccepted(
	t *testing.T,
) {
	line := "echo " + strings.Repeat("a", 1019)

	assert.Equal(t, exitFrame(0, ""), lastFrame(t, newTestHandlers(nil), line))
}

func TestHandlers_Exec_FinishedCommand_FreesItsSlot(
	t *testing.T,
) {
	h := newTestHandlers(nil)

	post(h, lineRequest("echo"))

	assert.Empty(t, h.slots)
}

func echoAuthorizationAsVersion(
	c *gin.Context,
) {
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"version": c.GetHeader("Authorization"), "build_id": "7", "api": gin.H{"latest": "v0"}}})
}

func TestHandlers_Exec_RealTree_RunsAgainstTheDaemonItselfAndIgnoresRedirectFlags(
	t *testing.T,
) {
	r := gin.New()
	r.GET("/versions", echoAuthorizationAsVersion)
	r.POST("/exec", New(logring.New(), "v9").Exec)
	srv := httptest.NewServer(r)
	defer srv.Close()
	body := `{"line":"version --server tcp://127.0.0.1:1 --context other --config /nonexistent/x"}`
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/exec", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer tok-1")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	output, err := io.ReadAll(resp.Body)

	require.NoError(t, err)
	assert.Contains(t, string(output), "client v9")
	assert.Contains(t, string(output), "daemon Bearer tok-1")
}

func TestOwnSession_Config_AlwaysFails(
	t *testing.T,
) {
	_, err := ownSession{}.Config()

	assert.Error(t, err)
}

func TestOwnSession_SpinnerAndIsTTY_NeverShowAnything(
	t *testing.T,
) {
	ran := false

	err := ownSession{}.Spinner(nil, "x", func() error {
		ran = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, ran)
	assert.False(t, ownSession{}.IsTTY())
}

func TestHandlers_Commands_ListsOnlyTheRunnableAllowedCommands(
	t *testing.T,
) {
	r := gin.New()
	r.GET("/commands", New(logring.New(), "v").Commands)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/commands", nil))

	var resp struct {
		Data struct{ Commands []struct{ Path []string } }
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	var paths []string
	for _, command := range resp.Data.Commands {
		paths = append(paths, strings.Join(command.Path, " "))
	}
	assert.Len(t, paths, 20)
	assert.Contains(t, paths, "arrow add")
	assert.NotContains(t, paths, "arrow", "a group is not runnable")
	assert.Contains(t, rec.Body.String(), `"usage":"install `)
}
