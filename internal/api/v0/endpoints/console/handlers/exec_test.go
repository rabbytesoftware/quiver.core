package console

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func testHandlers(started chan<- struct{}) *Handlers {
	h := New(logring.New(), "v")
	h.root = func(session.Session) *cobra.Command {
		root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
		for use, fn := range map[string]func(*cobra.Command) error{
			"echo": func(cmd *cobra.Command) error {
				fmt.Fprint(cmd.OutOrStdout(), "hello\n")
				fmt.Fprint(cmd.ErrOrStderr(), "careful\n")
				return nil
			},
			"fail":  func(*cobra.Command) error { return errors.New("boom") },
			"panic": func(*cobra.Command) error { panic("kaboom") },
			"flood": func(cmd *cobra.Command) error {
				for _, n := range []int{outputLimit - 1, 2, 7} {
					_, _ = cmd.OutOrStdout().Write([]byte(strings.Repeat("x", n)))
				}
				return nil
			},
			"wait": func(cmd *cobra.Command) error {
				close(started)
				<-cmd.Context().Done()
				return cmd.Context().Err()
			},
			"ask": func(cmd *cobra.Command) error {
				_, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				return err
			},
		} {
			root.AddCommand(clierr.AllowInConsole(&cobra.Command{Use: use, RunE: func(cmd *cobra.Command, _ []string) error { return fn(cmd) }})...)
		}
		root.AddCommand(&cobra.Command{Use: "hidden", Run: func(*cobra.Command, []string) {}})
		return root
	}
	return h
}

func post(h *Handlers, req *http.Request) *httptest.ResponseRecorder {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if id := c.GetHeader("X-Device"); id != "" {
			c.Set(middleware.DeviceContextKey, auth.Device{ID: id})
		}
	})
	r.POST("/exec", h.Exec)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func execReq(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/exec", strings.NewReader(body))
}

func execLine(line string) *http.Request {
	body, _ := json.Marshal(map[string]string{"line": line})
	return execReq(string(body))
}

func readFrames(t *testing.T, body io.Reader) (out []map[string]any) {
	t.Helper()
	for dec := json.NewDecoder(body); ; {
		var f map[string]any
		if err := dec.Decode(&f); err != nil {
			require.ErrorIs(t, err, io.EOF)
			return out
		}
		out = append(out, f)
	}
}

func captureLogs(t *testing.T) *strings.Builder {
	t.Helper()
	var buf strings.Builder
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func exit(code int, msg string) map[string]any {
	return map[string]any{"type": "exit", "code": float64(code), "error": msg}
}

func TestHandlers_Exec_StreamsOutFramesThenExactlyOneExitFrame(t *testing.T) {
	logs := captureLogs(t)
	req := execLine("echo hunter2")
	req.Header.Set("X-Device", "dev-7")

	rec := post(testHandlers(nil), req)

	assert.Equal(t, "application/x-ndjson", rec.Header().Get("Content-Type"))
	assert.Equal(t, []map[string]any{
		{"type": "out", "stream": "stdout", "data": "hello\n"},
		{"type": "out", "stream": "stderr", "data": "careful\n"},
		exit(0, ""),
	}, readFrames(t, rec.Body))
	for _, want := range []string{"component=console", "msg=exec ", "device=dev-7", `command="quiver echo"`, "code=0"} {
		assert.Contains(t, logs.String(), want)
	}
	assert.NotContains(t, logs.String(), "hunter2", "the audit record names the resolved command, never the raw line")
}

func TestHandlers_Exec_EndsWithTheOutcomeOfTheCommand(t *testing.T) {
	logs := captureLogs(t)
	h := testHandlers(nil)

	for line, want := range map[string]map[string]any{
		"fail":  exit(1, "boom"),
		"panic": exit(1, "internal error"),
		"ask":   exit(1, "EOF"),
	} {
		got := readFrames(t, post(h, execLine(line)).Body)

		assert.Equal(t, want, got[len(got)-1], line)
	}
	assert.Contains(t, logs.String(), "level=ERROR")
	assert.Contains(t, logs.String(), "kaboom")
}

func TestHandlers_Exec_CutsOutputPastTheLimitWithOneNote(t *testing.T) {
	got := readFrames(t, post(testHandlers(nil), execLine("flood")).Body)

	require.Len(t, got, 4, "two out frames, one note, one exit: the write after the cut is dropped")
	assert.Equal(t, outputLimit, len(got[0]["data"].(string))+len(got[1]["data"].(string)))
	assert.Contains(t, got[2]["data"], "output truncated")
	assert.Equal(t, exit(0, ""), got[3])
}

func TestHandlers_Exec_ADisconnectedClientCancelsTheCommand(t *testing.T) {
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *httptest.ResponseRecorder)
	go func() { done <- post(testHandlers(started), execLine("wait").WithContext(ctx)) }()

	<-started
	cancel()

	select {
	case rec := <-done:
		assert.Equal(t, exit(1, "context canceled"), readFrames(t, rec.Body)[0])
	case <-time.After(5 * time.Second):
		t.Fatal("the command was not cancelled")
	}
}

func TestHandlers_Exec_RefusesBeforeRunning(t *testing.T) {
	testCases := []struct {
		name string
		req  *http.Request
		busy bool
		want int
		msg  string
	}{
		{"not json", execReq("nope"), false, http.StatusBadRequest, "request body"},
		{"oversized body", execReq(`{"line":"` + strings.Repeat("a", 5000) + `"}`), false, http.StatusBadRequest, "request body"},
		{"bad line", execLine("echo; fail"), false, http.StatusBadRequest, "quoting is not supported"},
		{"unmarked command", execLine("hidden"), false, http.StatusForbidden, "hidden"},
		{"unknown command", execLine("nope"), false, http.StatusForbidden, "nope"},
		{"too many running", execLine("echo"), true, http.StatusTooManyRequests, "too many"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)
			h := testHandlers(nil)
			for range maxRunning {
				if tc.busy {
					h.slots <- struct{}{}
				}
			}

			rec := post(h, tc.req)

			assert.Equal(t, tc.want, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.msg)
			assert.NotContains(t, rec.Body.String(), `"type":"exit"`)
			assert.Equal(t, tc.want == http.StatusForbidden, strings.Contains(logs.String(), "level=WARN"))
		})
	}
}

func TestHandlers_Exec_FreesItsSlotWhenTheCommandEnds(t *testing.T) {
	h := testHandlers(nil)

	post(h, execLine("echo"))

	assert.Empty(t, h.slots)
}

func TestHandlers_Exec_RunsTheRealTreeAgainstTheDaemonItselfAndIgnoresRedirectFlags(t *testing.T) {
	var gotAuth string
	r := gin.New()
	r.GET("/versions", func(c *gin.Context) {
		gotAuth = c.GetHeader("Authorization")
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"version": "daemon-1.2.3", "build_id": "7", "api": gin.H{"latest": "v0"}}})
	})
	r.POST("/exec", New(logring.New(), "v9").Exec)
	srv := httptest.NewServer(r)
	defer srv.Close()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/exec", strings.NewReader(`{"line":"version --server tcp://127.0.0.1:1 --context other --config /nonexistent/x"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer tok-1")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)

	require.NoError(t, err)
	assert.Contains(t, string(body), "client v9")
	assert.Contains(t, string(body), "daemon daemon-1.2.3")
	assert.Equal(t, "Bearer tok-1", gotAuth)
}

func TestOwnSession_HasNoCLIConfigAndNeverShowsASpinner(t *testing.T) {
	s := ownSession{}
	ran := false

	_, cfgErr := s.Config()

	assert.Error(t, cfgErr)
	assert.NoError(t, s.Spinner(nil, "x", func() error { ran = true; return nil }))
	assert.True(t, ran)
	assert.False(t, s.IsTTY())
}

func TestHandlers_Commands_ListsOnlyTheRunnableAllowedCommands(t *testing.T) {
	r := gin.New()
	r.GET("/commands", New(logring.New(), "v").Commands)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/commands", nil))

	var resp struct {
		Data struct{ Commands []struct{ Path []string } }
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	var paths []string
	for _, c := range resp.Data.Commands {
		paths = append(paths, strings.Join(c.Path, " "))
	}
	assert.Len(t, paths, 20)
	assert.Contains(t, paths, "arrow add")
	assert.NotContains(t, paths, "arrow", "a group is not runnable")
	assert.Contains(t, rec.Body.String(), `"usage":"install `)
}
