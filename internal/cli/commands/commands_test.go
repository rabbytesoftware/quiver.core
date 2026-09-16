package commands_test

import (
	"bytes"
	"encoding/json"
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
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands"
)

const testNS = "github.com/user/app"

// ─── fake daemon ─────────────────────────────────────────────────────────────

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
		case path == "/v0/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case path == "/versions":
			ok(w, `{"version":"26.5","build_id":"83","api":{"supported":["v0"],"latest":"v0"}}`)
		case path == "/v0/arrow" && r.Method == http.MethodGet:
			ok(w, `[{"namespace":"`+testNS+`","name":"App","description":"An app","tags":["web"],"versions":[{"ref":"`+testNS+`@v1","version":"1.0.0","state":"ready","installed_at":"2026-01-01T00:00:00Z"}]}]`)
		case path == "/v0/arrow/github.com%2Fuser%2Fapp" && r.Method == http.MethodGet:
			ok(w, `{"namespace":"`+testNS+`","name":"App","version":"1.0.0","description":"An app","state":"ready","tags":["web"],"user_installed":true}`)
		case strings.HasSuffix(path, "/manifest") && strings.Contains(path, "arrow") && r.Method == http.MethodGet:
			ok(w, `{"namespace":"`+testNS+`","name":"App","targets":{"linux/amd64":{"lifecycle":{},"methods":{"backup":{"available_in":["ready"],"steps":[]},"seed-db":{"available_in":["ready"],"steps":[]}}}}}`)
		case r.Method == http.MethodPost || r.Method == http.MethodDelete || r.Method == http.MethodPatch:
			f.record(r)
			status := http.StatusAccepted
			if f.mutationStatus != 0 {
				status = f.mutationStatus
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"success":true}`))
		case path == "/v0/collection" && r.Method == http.MethodGet:
			ok(w, `[{"namespace":"github.com/user/col","name":"Col","description":"d","tags":[],"arrow_count":2,"followed":true}]`)
		case path == "/v0/collection/github.com%2Fuser%2Fcol" && r.Method == http.MethodGet:
			ok(w, `{"namespace":"github.com/user/col","name":"Col","description":"d","maintainers":[],"tags":[],"arrows":[{"namespace":"`+testNS+`","name":"App"}],"followed":true}`)
		case path == "/v0/runtime" && r.Method == http.MethodGet:
			ok(w, `[{"namespace":"`+testNS+`","state":"running","active_run":{"method":"_execute","pid":42}},{"namespace":"github.com/user/idle","state":"ready"}]`)
		case path == "/v0/runtime/github.com%2Fuser%2Fapp" && r.Method == http.MethodGet:
			if f.runtimeDetail != "" {
				ok(w, f.runtimeDetail)
				return
			}
			ok(w, `{"namespace":"`+testNS+`","state":"running","active_run":{"method":"_execute","pid":42}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"error":"not found"}`))
		}
	})
}

// ─── harness ─────────────────────────────────────────────────────────────────

func runCLI(t *testing.T, f *fakeDaemon, args ...string) (string, error) {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.Attach(root, commands.Deps{
		Version: "test",
		IsTTY:   func() bool { return false },
	})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	cfgPath := filepath.Join(t.TempDir(), "cli.yaml")
	root.SetArgs(append(args, "--server", srv.URL, "--config", cfgPath))

	err := root.Execute()
	return out.String(), err
}

// runCLIConfig runs context commands against an isolated config without a server.
func runCLIConfig(t *testing.T, cfgPath string, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.Attach(root, commands.Deps{Version: "test", IsTTY: func() bool { return false }})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append(args, "--config", cfgPath))

	err := root.Execute()
	return out.String(), err
}

// ─── namespace dispatch ──────────────────────────────────────────────────────

func TestDispatch_CustomMethod(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "ready", LastReturn: &apidto.ReturnDTO{Method: "backup", Outcome: "success"}},
	}}

	_, err := runCLI(t, f, testNS, "backup")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/backup")
}

func TestDispatch_BareNamespaceShowsHelpPanel(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, testNS)
	require.NoError(t, err)
	assert.Contains(t, out, testNS)
	assert.Contains(t, out, "install")
	assert.Contains(t, out, "refresh", "panel should surface arrow refresh")
}

func TestRootHelp_ExplainsRunningArrows(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, _ := runCLI(t, f, "--help")
	assert.Contains(t, out, "quiver <namespace>",
		"top-level help should explain the namespace dispatch form")
	assert.Contains(t, out, "custom", "top-level help should mention custom methods")
}

func TestDispatch_UnknownBareWordErrors(t *testing.T) {
	f := &fakeDaemon{t: t}
	_, err := runCLI(t, f, "frobnicate")
	assert.Error(t, err)
}

func TestNamespacePanel_ListsCustomMethodsFromTheManifest(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, testNS, "-o", "json")
	require.NoError(t, err)

	var panel commands.Panel
	require.NoError(t, json.Unmarshal([]byte(out), &panel))
	assert.Equal(t, []string{"backup", "seed-db"}, panel.Methods)
}

func TestNamespacePanel_OmitsMethodsWhenTheDaemonCannotAnswer(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "cli.yaml")

	out, err := runCLIConfig(t, cfgPath, testNS, "-o", "json")
	require.NoError(t, err)

	var panel commands.Panel
	require.NoError(t, json.Unmarshal([]byte(out), &panel))
	assert.Empty(t, panel.Methods)
	assert.NotEmpty(t, panel.Lifecycle, "lifecycle verbs do not depend on the daemon")
}

func TestNamespacePanel_OmitsMethodsWhenSessionFails(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "cli.yaml")

	// Use a non-existent context so cfg.Resolve fails, which makes
	// a.session fail before the manifest fetch can even be attempted.
	out, err := runCLIConfig(t, cfgPath, testNS, "--context", "nonexistent", "-o", "json")
	require.NoError(t, err, "command should exit 0 even though context resolution fails")

	var panel commands.Panel
	require.NoError(t, json.Unmarshal([]byte(out), &panel))
	assert.Empty(t, panel.Methods)
	assert.NotEmpty(t, panel.Lifecycle, "lifecycle verbs do not depend on session")
}

// ─── discovery ───────────────────────────────────────────────────────────────

func TestList_TableShowsArrowsAndCollections(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "list", "-o", "table")
	require.NoError(t, err)
	assert.Contains(t, out, testNS)
	assert.Contains(t, out, "github.com/user/col")
	assert.Contains(t, out, "ARROWS")
	assert.Contains(t, out, "COLLECTIONS")
}

func TestList_JSONIsCombinedObject(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "list", "-o", "json")
	require.NoError(t, err)

	var doc struct {
		Arrows      []map[string]any `json:"arrows"`
		Collections []map[string]any `json:"collections"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &doc))
	assert.Len(t, doc.Arrows, 1)
	assert.Len(t, doc.Collections, 1)
}

func TestList_FilterGlob(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "list", "-o", "json", "-F", "github.com/other/*")
	require.NoError(t, err)
	assert.NotContains(t, out, testNS)
}

func TestSearch_MatchesPattern(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "search", "github.com/user/*")
	require.NoError(t, err)
	assert.Contains(t, out, testNS)
}

func TestSearch_NoMatches(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "search", "gitlab.com/*")
	require.NoError(t, err)
	assert.Contains(t, out, "0")
}

func TestInfo_ShowsDetail(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "info", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "App")
	assert.Contains(t, out, "ready")
}

func TestInfo_ManifestFlagReturnsRaw(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "info", testNS, "--manifest")
	require.NoError(t, err)
	assert.Contains(t, out, `"targets"`)
}

func TestMethods_ListsCustomMethods(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "methods", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "backup")
	assert.Contains(t, out, "seed-db")
	assert.NotContains(t, out, "install ·")
}

func TestMethods_IncludeBuiltins(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "methods", testNS, "--include-builtins")
	require.NoError(t, err)
	assert.Contains(t, out, "install")
	assert.Contains(t, out, "backup")
}
