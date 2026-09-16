package runtime_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
)

const testNS = "github.com/user/app"

// ─── fixtures ────────────────────────────────────────────────────────────────

// newSession builds a session.Session pointed at server. Mirrors the pattern
// in commands/system/system_test.go and commands/collection/collection_test.go.
func newSession(t *testing.T, server string, tty bool) session.Session {
	t.Helper()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cli.yaml")
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.NoError(t, cfg.Add(config.Context{Name: "test", Server: server}, true))

	return session.New(
		session.Deps{IsTTYFunc: func() bool { return tty }},
		&session.Flags{Config: cfgPath},
	)
}

// fakeDaemon serves the runtime-only endpoints the runtime package's
// commands call: the mutation methods (install/execute/stop/uninstall/
// update/custom), the runtime listing/detail reads, and the runtime
// WebSocket channel install/run/watch subscribe to.
type fakeDaemon struct {
	t *testing.T
	// wsScript frames are pushed to any runtime WS subscriber.
	wsScript []apidto.ArrowRuntimeDTO
	// mutationStatus overrides the status returned for POST/PATCH/DELETE
	// mutations; zero means the default 202 Accepted.
	mutationStatus int
	// runtimeDetail overrides the GET /v0/runtime/{ns} payload; empty means
	// the default running runtime.
	runtimeDetail string
	// fail makes every request return 500, for daemon-error-propagation
	// tests.
	fail bool

	mu    sync.Mutex
	posts []string // recorded "METHOD path" of mutations
}

func (f *fakeDaemon) record(r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posts = append(f.posts, r.Method+" "+r.URL.EscapedPath())
}

func (f *fakeDaemon) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.posts...)
}

func ok(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true,"data":` + data + `}`))
}

func (f *fakeDaemon) handler() http.Handler {
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.fail {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))
			return
		}

		path := r.URL.EscapedPath()

		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			conn, err := up.Upgrade(w, r, nil)
			require.NoError(f.t, err)
			defer func() { _ = conn.Close() }()
			for _, evt := range f.wsScript {
				raw, _ := json.Marshal(evt)
				_ = conn.WriteMessage(websocket.TextMessage, raw)
			}
			time.Sleep(200 * time.Millisecond)
			return
		}

		switch {
		case path == "/v0/runtime" && r.Method == http.MethodGet:
			ok(w, `[{"namespace":"`+testNS+`","state":"running","active_run":{"method":"_execute","pid":42}},`+
				`{"namespace":"github.com/user/idle","state":"ready"}]`)
		case path == "/v0/runtime/github.com%2Fuser%2Fapp" && r.Method == http.MethodGet:
			if f.runtimeDetail != "" {
				ok(w, f.runtimeDetail)
				return
			}
			ok(w, `{"namespace":"`+testNS+`","state":"running","active_run":{"method":"_execute","pid":42}}`)
		case r.Method == http.MethodPost || r.Method == http.MethodDelete || r.Method == http.MethodPatch:
			f.record(r)
			status := http.StatusAccepted
			if f.mutationStatus != 0 {
				status = f.mutationStatus
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"error":"not found"}`))
		}
	})
}

func installScript() []apidto.ArrowRuntimeDTO {
	title := "Fetching binary"
	return []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "installing", ActiveRun: &apidto.RunRecordDTO{
			Method: "_install",
			Steps:  []apidto.StepProgressDTO{{Index: 0, Status: "running", Title: title, Type: "fetch"}},
		}},
		{Namespace: testNS, State: "installing", ActiveRun: &apidto.RunRecordDTO{
			Method: "_install",
			Steps:  []apidto.StepProgressDTO{{Index: 0, Status: "completed", Title: title, Type: "fetch"}},
		}},
		{Namespace: testNS, State: "ready", LastReturn: &apidto.ReturnDTO{Method: "_install", Outcome: "success"}},
	}
}

// runRuntime runs the runtime command tree against sess, in outputFormat
// ("" means the runner's default: json on a non-TTY session), with an
// optional stdin.
func runRuntime(
	t *testing.T, sess session.Session, outputFormat string, stdin io.Reader, args ...string,
) (string, error) {
	t.Helper()

	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(runtime.New(sess, runner.New(&runner.Flags{Output: outputFormat})).Cmd()...)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if stdin != nil {
		root.SetIn(stdin)
	}
	root.SetArgs(args)

	err := root.Execute()

	return out.String(), err
}

// runCLI is the common case of runRuntime: a fresh httptest server around f,
// no stdin override, in outputFormat ("" means the runner's default: json on
// this non-TTY session).
func runCLI(t *testing.T, f *fakeDaemon, outputFormat string, args ...string) (string, error) {
	t.Helper()

	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	return runRuntime(t, newSession(t, srv.URL, false), outputFormat, nil, args...)
}

// ─── Cmd ─────────────────────────────────────────────────────────────────────

func TestCmd_ReturnsEightCommands(t *testing.T) {
	got := runtime.New(newSession(t, "unix:///unused.sock", false), runner.New(&runner.Flags{})).Cmd()

	require.Len(t, got, 8)

	names := make([]string, 0, len(got))
	for _, c := range got {
		names = append(names, strings.Fields(c.Use)[0])
	}
	assert.Equal(t, []string{
		"install", "run", "stop", "uninstall", "update", "ps", "status", "watch",
	}, names)
}

// ─── session + config errors ─────────────────────────────────────────────────

func TestSessionErrors_AllCommands(t *testing.T) {
	testCases := [][]string{
		{"ps"},
		{"status"},
		{"status", testNS},
		{"install", testNS},
		{"install", testNS, "--detach"},
		{"run", testNS},
		{"stop", testNS},
		{"update", testNS},
		{"uninstall", testNS, "--yes"},
		{"watch", testNS},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := runRuntime(t, newSession(t, "ftp://nope", false), "json", nil, args...)
			assert.Error(t, err)
		})
	}
}

func TestCommands_DaemonErrorsPropagate(t *testing.T) {
	testCases := [][]string{
		{"ps"},
		{"status"},
		{"status", testNS},
		{"install", testNS, "--detach"},
		{"install", testNS},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := runCLI(t, &fakeDaemon{t: t, fail: true}, "json", args...)
			assert.Error(t, err)
		})
	}
}

// ─── namespace validation ────────────────────────────────────────────────────

func TestValidNS_RejectsGarbage(t *testing.T) {
	for _, args := range [][]string{
		{"install", "notanamespace"},
		{"status", "notanamespace"},
	} {
		_, err := runCLI(t, &fakeDaemon{t: t}, "json", args...)
		require.Error(t, err, "args: %v", args)
	}
}

func TestStatusWatch_RejectsInvalidNamespace(t *testing.T) {
	_, err := runCLI(t, &fakeDaemon{t: t}, "json", "status", "notanamespace", "-w")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid namespace")
}

// ─── RunMethod ───────────────────────────────────────────────────────────────

// RunMethod backs the root command's bare `quiver <namespace> <method>`
// dispatch, which has no cobra command of its own — exercised directly here
// since nothing in this package's own Cmd() tree calls it.
func TestRunMethod_DispatchesCustomMethod(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "ready", LastReturn: &apidto.ReturnDTO{Method: "backup", Outcome: "success"}},
	}}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		return runtime.RunMethod(
			newSession(t, srv.URL, false), runner.New(&runner.Flags{Output: "json"}),
			cmd, testNS, "backup", false, nil,
		)
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	require.NoError(t, root.Execute())
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/backup")
}
