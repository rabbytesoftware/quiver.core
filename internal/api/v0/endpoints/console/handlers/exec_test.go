package console_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	handlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/console/handlers"
	"github.com/rabbytesoftware/quiver.core/internal/console/command"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

type fakeInvocation struct {
	run func(ctx context.Context, stdout, stderr io.Writer) command.Result
}

func (f fakeInvocation) Run(
	ctx context.Context,
	stdout io.Writer,
	stderr io.Writer,
) command.Result {
	return f.run(ctx, stdout, stderr)
}

type fakeExecutor struct {
	mu        sync.Mutex
	tokens    [][]string
	bearers   []string
	prepare   func(tokens []string, bearer string) (command.Invocation, error)
	describes []command.CommandInfo
}

func (f *fakeExecutor) Prepare(
	tokens []string,
	bearer string,
) (command.Invocation, error) {
	f.mu.Lock()
	f.tokens = append(f.tokens, tokens)
	f.bearers = append(f.bearers, bearer)
	f.mu.Unlock()
	return f.prepare(tokens, bearer)
}

func (f *fakeExecutor) Commands() []command.CommandInfo {
	return f.describes
}

func echoing(
	out string,
	result command.Result,
) *fakeExecutor {
	return &fakeExecutor{prepare: func([]string, string) (command.Invocation, error) {
		return fakeInvocation{run: func(_ context.Context, stdout, stderr io.Writer) command.Result {
			_, _ = stdout.Write([]byte(out))
			_, _ = stderr.Write([]byte("warn\n"))
			return result
		}}, nil
	}}
}

func newRouter(
	exec command.Executor,
	setup func(*gin.Engine),
	opts ...handlers.Option,
) *gin.Engine {
	r := gin.New()
	if setup != nil {
		setup(r)
	}
	h := handlers.New(logring.New(10), exec, opts...)
	r.POST("/console/exec", h.Exec)
	r.GET("/console/commands", h.Commands)
	return r
}

func post(
	r http.Handler,
	body string,
	header ...string,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/console/exec", strings.NewReader(body))
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func frames(
	t *testing.T,
	body string,
) []map[string]any {
	t.Helper()

	var out []map[string]any
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var frame map[string]any
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &frame), scanner.Text())
		out = append(out, frame)
	}
	return out
}

func captureSlog(
	t *testing.T,
) logring.Ring {
	t.Helper()

	ring := logring.New(50)
	previous := slog.Default()
	slog.SetDefault(slog.New(ring.Tee(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return ring
}

func TestExec_StreamsOutAndExitFrames(t *testing.T) {
	r := newRouter(echoing("hello\n", command.Result{Code: 0}), nil)

	w := post(r, `{"line":"list"}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/x-ndjson", w.Header().Get("Content-Type"))
	got := frames(t, w.Body.String())
	require.Len(t, got, 3)
	assert.Equal(t, map[string]any{"type": "out", "stream": "stdout", "data": "hello\n"}, got[0])
	assert.Equal(t, map[string]any{"type": "out", "stream": "stderr", "data": "warn\n"}, got[1])
	assert.Equal(t, map[string]any{"type": "exit", "code": float64(0), "error": ""}, got[2])
}

func TestExec_ReportsAFailureOnTheExitFrame(t *testing.T) {
	r := newRouter(echoing("", command.Result{Code: 2, Err: "usage: x"}), nil)

	got := frames(t, post(r, `{"line":"list"}`).Body.String())

	last := got[len(got)-1]
	assert.Equal(t, float64(2), last["code"])
	assert.Equal(t, "usage: x", last["error"])
}

func TestExec_ExactlyOneExitFrameEndsTheResponse(t *testing.T) {
	r := newRouter(echoing("x", command.Result{}), nil)

	got := frames(t, post(r, `{"line":"list"}`).Body.String())

	exits := 0
	for _, f := range got {
		if f["type"] == "exit" {
			exits++
		}
	}
	assert.Equal(t, 1, exits)
	assert.Equal(t, "exit", got[len(got)-1]["type"])
}

func TestExec_TokenizesTheLineAndNeverPassesItToAShell(t *testing.T) {
	exec := echoing("", command.Result{})
	r := newRouter(exec, nil)

	post(r, `{"line":"run x --data \"k=a b\" ; rm -rf / $(id)"}`)

	require.Len(t, exec.tokens, 1)
	assert.Equal(t, []string{"run", "x", "--data", "k=a b", ";", "rm", "-rf", "/", "$(id)"}, exec.tokens[0])
}

func TestExec_RejectsBadRequests(t *testing.T) {
	cases := map[string]string{
		"not json":               `nope`,
		"wrong type":             `{"line":5}`,
		"unknown field":          `{"line":"list","env":"X=1"}`,
		"missing line":           `{}`,
		"empty line":             `{"line":""}`,
		"blank line":             `{"line":"   "}`,
		"newline in line":        `{"line":"list\nrun x"}`,
		"nul in line":            `{"line":"list\u0000"}`,
		"unterminated quote":     `{"line":"run \"x"}`,
		"line over 1024 bytes":   `{"line":"` + strings.Repeat("a", 1025) + `"}`,
		"more than 64 arguments": `{"line":"` + strings.Repeat("a ", 65) + `"}`,
		"body over the cap":      `{"line":"list","pad":"` + strings.Repeat("a", 5000) + `"}`,
	}
	exec := echoing("", command.Result{})
	r := newRouter(exec, nil)

	for name, body := range cases {
		w := post(r, body)
		assert.Equal(t, http.StatusBadRequest, w.Code, name)
	}
	assert.Empty(t, exec.tokens, "a rejected line must never reach the executor")
}

func TestExec_DeniedCommandsAre403WithTheReason(t *testing.T) {
	exec := &fakeExecutor{prepare: func([]string, string) (command.Invocation, error) {
		return nil, &command.DeniedError{Command: "quiver daemon", Reason: `the command "daemon" is not available in the console`}
	}}
	r := newRouter(exec, nil)

	w := post(r, `{"line":"daemon"}`)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), `the command \"daemon\" is not available in the console`)
	assert.NotContains(t, w.Header().Get("Content-Type"), "ndjson")
}

func TestExec_UnavailableIs503AndUnexpectedErrorsAre500WithoutLeaking(t *testing.T) {
	unavailable := &fakeExecutor{prepare: func([]string, string) (command.Invocation, error) { return nil, command.ErrUnavailable }}
	broken := &fakeExecutor{prepare: func([]string, string) (command.Invocation, error) { return nil, errors.New("secret detail") }}

	assert.Equal(t, http.StatusServiceUnavailable, post(newRouter(unavailable, nil), `{"line":"list"}`).Code)
	w := post(newRouter(broken, nil), `{"line":"list"}`)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "secret detail")
}

func TestExec_ForwardsTheCallersBearerToken(t *testing.T) {
	exec := echoing("", command.Result{})
	r := newRouter(exec, nil)

	post(r, `{"line":"list"}`, "Authorization", "Bearer abc123")
	post(r, `{"line":"list"}`)
	post(r, `{"line":"list"}`, "Authorization", "Basic xyz")

	assert.Equal(t, []string{"abc123", "", ""}, exec.bearers)
}

func blocking(
	started chan<- struct{},
	release <-chan struct{},
) *fakeExecutor {
	return &fakeExecutor{prepare: func([]string, string) (command.Invocation, error) {
		return fakeInvocation{run: func(ctx context.Context, _, _ io.Writer) command.Result {
			started <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
			}
			return command.Result{}
		}}, nil
	}}
}

func TestExec_LimitsConcurrentCommandsPerDeviceAndReleasesSlots(t *testing.T) {
	started := make(chan struct{}, 10)
	release := make(chan struct{})
	exec := blocking(started, release)
	setDevice := func(r *gin.Engine) {
		r.Use(func(c *gin.Context) {
			c.Set(middleware.DeviceContextKey, auth.Device{ID: c.GetHeader("X-Device")})
		})
	}
	r := newRouter(exec, setDevice, handlers.WithLimiter(handlers.NewExecLimiter(2, 3)))
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			post(r, `{"line":"list"}`, "X-Device", "a")
		}()
		<-started
	}

	third := post(r, `{"line":"list"}`, "X-Device", "a")
	assert.Equal(t, http.StatusTooManyRequests, third.Code)

	wg.Add(1)
	go func() {
		defer wg.Done()
		post(r, `{"line":"list"}`, "X-Device", "b")
	}()
	<-started

	global := post(r, `{"line":"list"}`, "X-Device", "c")
	assert.Equal(t, http.StatusTooManyRequests, global.Code, "the global cap must apply across devices")

	close(release)
	wg.Wait()
	again := post(r, `{"line":"list"}`, "X-Device", "a")
	assert.Equal(t, http.StatusOK, again.Code, "slots must be released when commands finish")
}

func TestExec_ADeniedCommandDoesNotHoldASlot(t *testing.T) {
	exec := &fakeExecutor{prepare: func([]string, string) (command.Invocation, error) {
		return nil, &command.DeniedError{Reason: "no"}
	}}
	r := newRouter(exec, nil, handlers.WithLimiter(handlers.NewExecLimiter(1, 1)))

	for i := 0; i < 5; i++ {
		assert.Equal(t, http.StatusForbidden, post(r, `{"line":"daemon"}`).Code)
	}
}

func TestExec_CapsOutputAndStillDeliversTheExitFrame(t *testing.T) {
	big := strings.Repeat("x", 1000)
	exec := &fakeExecutor{prepare: func([]string, string) (command.Invocation, error) {
		return fakeInvocation{run: func(_ context.Context, stdout, _ io.Writer) command.Result {
			for i := 0; i < 100; i++ {
				_, _ = stdout.Write([]byte(big))
			}
			return command.Result{Code: 0}
		}}, nil
	}}
	r := newRouter(exec, nil, handlers.WithOutputLimit(4096))

	got := frames(t, post(r, `{"line":"list"}`).Body.String())

	total := 0
	notices := 0
	for _, f := range got {
		if f["type"] != "out" {
			continue
		}
		data, _ := f["data"].(string)
		if strings.Contains(data, "output truncated") {
			notices++
			continue
		}
		total += len(data)
	}
	assert.LessOrEqual(t, total, 4096)
	assert.Equal(t, 1, notices)
	assert.Equal(t, "exit", got[len(got)-1]["type"])
	assert.Equal(t, float64(0), got[len(got)-1]["code"], "truncation must not kill or fail the command")
}

func TestExec_TimesOutACommandAndSaysSo(t *testing.T) {
	exec := &fakeExecutor{prepare: func([]string, string) (command.Invocation, error) {
		return fakeInvocation{run: func(ctx context.Context, _, _ io.Writer) command.Result {
			<-ctx.Done()
			return command.Result{Code: 1, Err: ctx.Err().Error()}
		}}, nil
	}}
	r := newRouter(exec, nil, handlers.WithExecTimeout(30*time.Millisecond))

	got := frames(t, post(r, `{"line":"list"}`).Body.String())

	last := got[len(got)-1]
	assert.Equal(t, "exit", last["type"])
	assert.Equal(t, float64(1), last["code"])
	assert.Contains(t, last["error"], "timed out after 30ms")
}

func TestExec_CancelsTheCommandWhenTheClientGoesAway(t *testing.T) {
	cancelled := make(chan struct{})
	started := make(chan struct{})
	exec := &fakeExecutor{prepare: func([]string, string) (command.Invocation, error) {
		return fakeInvocation{run: func(ctx context.Context, _, _ io.Writer) command.Result {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return command.Result{Code: 1}
		}}, nil
	}}
	r := newRouter(exec, nil)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/console/exec", strings.NewReader(`{"line":"list"}`)).WithContext(ctx)

	go r.ServeHTTP(httptest.NewRecorder(), req)
	<-started
	cancel()

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("the command was not cancelled")
	}
}

func TestExec_OutputWrittenAfterTheCommandReturnedIsDiscarded(t *testing.T) {
	late := make(chan io.Writer, 1)
	exec := &fakeExecutor{prepare: func([]string, string) (command.Invocation, error) {
		return fakeInvocation{run: func(_ context.Context, stdout, _ io.Writer) command.Result {
			late <- stdout
			return command.Result{}
		}}, nil
	}}
	r := newRouter(exec, nil)

	w := post(r, `{"line":"list"}`)
	n, err := (<-late).Write([]byte("too late"))

	require.NoError(t, err)
	assert.Equal(t, len("too late"), n)
	assert.NotContains(t, w.Body.String(), "too late")
}

func TestExec_AuditsEveryRunWithTheDeviceAndARedactedLine(t *testing.T) {
	ring := captureSlog(t)
	setDevice := func(r *gin.Engine) {
		r.Use(func(c *gin.Context) { c.Set(middleware.DeviceContextKey, auth.Device{ID: "dev-7"}) })
	}
	r := newRouter(echoing("", command.Result{Code: 0}), setDevice)

	post(r, `{"line":"run x --data token=hunter2 --token abc"}`)

	var audit *logring.Record
	for _, rec := range ring.Snapshot(0, slog.LevelDebug, 50) {
		if rec.Msg == "exec" {
			audit = &rec
		}
	}
	require.NotNil(t, audit)
	assert.Equal(t, "console", audit.Component)
	assert.Equal(t, "dev-7", audit.Fields["device"])
	assert.Equal(t, "run x --data token=[redacted] --token [redacted]", audit.Fields["line"])
	assert.Equal(t, int64(0), audit.Fields["code"])
	assert.NotNil(t, audit.Fields["took"])
	assert.NotContains(t, audit.Fields["line"], "hunter2")
}

func TestExec_AuditsLocalCallersAsLocal(t *testing.T) {
	ring := captureSlog(t)
	r := newRouter(echoing("", command.Result{}), nil)

	post(r, `{"line":"list"}`)

	for _, rec := range ring.Snapshot(0, slog.LevelDebug, 50) {
		if rec.Msg == "exec" {
			assert.Equal(t, "local", rec.Fields["device"])
			return
		}
	}
	t.Fatal("no audit record")
}

func TestExec_LogsDeniedAttemptsWithoutSecrets(t *testing.T) {
	ring := captureSlog(t)
	exec := &fakeExecutor{prepare: func([]string, string) (command.Invocation, error) {
		return nil, &command.DeniedError{Reason: "not available"}
	}}
	r := newRouter(exec, nil)

	post(r, `{"line":"context add x --token supersecret"}`)

	for _, rec := range ring.Snapshot(0, slog.LevelDebug, 50) {
		if rec.Msg == "exec denied" {
			assert.NotContains(t, rec.Fields["line"], "supersecret")
			assert.Equal(t, "warn", rec.Level)
			return
		}
	}
	t.Fatal("no denial record")
}

func TestCommands_ReturnsTheExecutorsList(t *testing.T) {
	exec := &fakeExecutor{describes: []command.CommandInfo{{Path: []string{"arrow", "add"}, Short: "s", Usage: "arrow add <ns>"}}}
	r := newRouter(exec, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/console/commands", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Commands []command.CommandInfo `json:"commands"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.Success)
	require.Len(t, body.Data.Commands, 1)
	assert.Equal(t, []string{"arrow", "add"}, body.Data.Commands[0].Path)
}
