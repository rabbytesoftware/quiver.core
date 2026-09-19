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
//
// Only the endpoints the dispatch/panel tests below exercise: custom-method
// dispatch (a WS subscribe plus the mutation POST) and the bare-namespace
// panel's manifest fetch. Every discovery-specific endpoint (arrow/collection
// listing, runtime detail, health, versions) lives with the tests that need
// it in commands/discovery, commands/runtime, and commands/system.

type fakeDaemon struct {
	t *testing.T
	// wsScript frames are pushed to any runtime WS subscriber.
	wsScript []apidto.ArrowRuntimeDTO
	// manifest overrides the arrow manifest payload; empty means the
	// default manifest with two custom methods.
	manifest string

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
		case strings.HasSuffix(path, "/manifest") && strings.Contains(path, "arrow") && r.Method == http.MethodGet:
			if f.manifest != "" {
				ok(w, f.manifest)
				return
			}
			ok(w, `{"namespace":"`+testNS+`","name":"App","targets":{"linux/amd64":{"lifecycle":{},"methods":{"backup":{"available_in":["ready"],"steps":[]},"seed-db":{"available_in":["ready"],"steps":[]}}}}}`)
		case r.Method == http.MethodPost || r.Method == http.MethodDelete || r.Method == http.MethodPatch:
			f.record(r)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"success":true}`))
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
	commands.New(commands.Deps{
		Version: "test",
		IsTTY:   func() bool { return false },
	}).Attach(root)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	cfgPath := filepath.Join(t.TempDir(), "cli.yaml")
	root.SetArgs(append(args, "--server", srv.URL, "--config", cfgPath))

	err := root.Execute()
	return out.String(), err
}

// runCLIConfig runs the dispatch commands against an isolated config without a server.
func runCLIConfig(t *testing.T, cfgPath string, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.New(commands.Deps{Version: "test", IsTTY: func() bool { return false }}).Attach(root)

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

func TestDispatch_CustomMethodWithRootFlags(t *testing.T) {
	f := &fakeDaemon{t: t}
	out, err := runCLI(t, f, testNS, "backup", "--detach", "--data", "k=v")
	require.NoError(t, err)
	assert.Contains(t, out, "started, not waiting")
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
	assert.Equal(t, 2, commands.ExitCode(err))
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
	// t.sess.Client fail before the manifest fetch can even be attempted.
	out, err := runCLIConfig(t, cfgPath, testNS, "--context", "nonexistent", "-o", "json")
	require.NoError(t, err, "command should exit 0 even though context resolution fails")

	var panel commands.Panel
	require.NoError(t, json.Unmarshal([]byte(out), &panel))
	assert.Empty(t, panel.Methods)
	assert.NotEmpty(t, panel.Lifecycle, "lifecycle verbs do not depend on session")
}

func TestNamespacePanel_OmitsMethodsWhenManifestIsUnparsable(t *testing.T) {
	f := &fakeDaemon{t: t, manifest: `"not-an-object"`}

	out, err := runCLI(t, f, testNS, "-o", "json")
	require.NoError(t, err, "namespacePanel swallows namespaceMethods' parse failure too")

	var panel commands.Panel
	require.NoError(t, json.Unmarshal([]byte(out), &panel))
	assert.Empty(t, panel.Methods)
}
