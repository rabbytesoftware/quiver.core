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
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
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

// ─── lifecycle ───────────────────────────────────────────────────────────────

func TestInstall_HappyPathPlainOutput(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: installScript()}

	out, err := runCLI(t, f, "install", testNS)
	require.NoError(t, err)

	// Piped, install emits the run payload. The steps it executed are in it,
	// which is what the old line-per-transition output carried.
	var run output.Run
	require.NoError(t, json.Unmarshal([]byte(out), &run), "run payload: %s", out)

	assert.Equal(t, "install", run.Method)
	assert.Equal(t, testNS, run.Subject)
	assert.Equal(t, "success", run.Outcome)
	assert.Equal(t, "ready", run.State)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/install")
}

func TestInstall_NoOpAlreadyInstalled(t *testing.T) {
	f := &fakeDaemon{t: t, mutationStatus: http.StatusOK}

	out, err := runCLI(t, f, "install", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "already installed, nothing to do")
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/install")
}

func TestInstall_FailureReturnsError(t *testing.T) {
	msg := "fetch: 404"
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "absent", LastReturn: &apidto.ReturnDTO{
			Method: "_install", Outcome: "failed",
			Steps: []apidto.StepProgressDTO{{Index: 0, Status: "failed", Error: &msg, Type: "fetch"}},
		}},
	}}

	out, err := runCLI(t, f, "install", testNS)
	require.Error(t, err)

	// A failed run writes no payload: the error is the result, and a payload
	// on stdout would tell a script the opposite.
	assert.Empty(t, out)

	// The error names the step that stopped and what it said, rather than
	// just reporting the daemon's one-word verdict.
	assert.Contains(t, err.Error(), "install "+testNS)
	assert.Contains(t, err.Error(), "failed")
	assert.Contains(t, err.Error(), "step 1/1")
	assert.Contains(t, err.Error(), msg)
	assert.Contains(t, err.Error(), "state absent")
}

func TestInstall_DetachSkipsWait(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "install", testNS, "--detach")
	require.NoError(t, err)
	assert.Contains(t, out, "started, not waiting")
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/install")
}

func TestRun_MapsToExecuteEndpoint(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "running", LastReturn: &apidto.ReturnDTO{Method: "_execute", Outcome: "success"}},
	}}

	_, err := runCLI(t, f, "run", testNS)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/execute")
}

func TestStop_PostsStop(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "ready", LastReturn: &apidto.ReturnDTO{Method: "_stop", Outcome: "success"}},
	}}

	_, err := runCLI(t, f, "stop", testNS)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/stop")
}

func TestUninstall_YesFlagSkipsConfirmation(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "absent", LastReturn: &apidto.ReturnDTO{Method: "_uninstall", Outcome: "success"}},
	}}

	_, err := runCLI(t, f, "uninstall", testNS, "--yes")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/uninstall")
}

func TestUninstall_YesShorthandSkipsConfirmation(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "absent", LastReturn: &apidto.ReturnDTO{Method: "_uninstall", Outcome: "success"}},
	}}

	_, err := runCLI(t, f, "uninstall", testNS, "-y")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/uninstall")
}

func TestArrowRemove_YesShorthandSkipsConfirmation(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "arrow", "remove", testNS, "-y")
	require.NoError(t, err)
	assertMutation(t, out, "remove", testNS)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "DELETE /v0/arrow/github.com%2Fuser%2Fapp")
}

func TestArrowRefresh_PatchesManifest(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "arrow", "refresh", testNS)
	require.NoError(t, err)
	assertMutation(t, out, "refresh", testNS)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "PATCH /v0/arrow/github.com%2Fuser%2Fapp")
}

func TestUpdate_PostsUpdate(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "ready", LastReturn: &apidto.ReturnDTO{Method: "_update", Outcome: "success"}},
	}}

	_, err := runCLI(t, f, "update", testNS)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/update")
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

// ─── observation ─────────────────────────────────────────────────────────────

func TestPs_ShowsRunning(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "ps")
	require.NoError(t, err)
	assert.Contains(t, out, testNS)
	assert.Contains(t, out, "running")
	assert.NotContains(t, out, "github.com/user/idle")
}

func TestPs_AllIncludesIdle(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "ps", "--all")
	require.NoError(t, err)
	assert.Contains(t, out, "github.com/user/idle")
}

func TestStatus_SingleArrow(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "status", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "running")
	assert.Contains(t, out, "42")
}

func TestStatus_AllArrows(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "status")
	require.NoError(t, err)
	assert.Contains(t, out, testNS)
	assert.Contains(t, out, "github.com/user/idle")
}

// A run that has already finished is the case the manual points users at:
// "if a background install failed while you were away, this is where you find
// out what went wrong and at which step". The steps live on last_return, not
// on active_run, which is nil by then.
func TestStatus_ShowsFailedStepOfCompletedRun(t *testing.T) {
	f := &fakeDaemon{t: t, runtimeDetail: `{"namespace":"` + testNS + `","state":"absent",` +
		`"last_return":{"method":"_install","outcome":"failure","steps":[` +
		`{"index":1,"status":"completed","title":"Resolve dependencies","type":"noop"},` +
		`{"index":2,"status":"failed","title":"Downloading FFmpeg build","type":"fetch","error":"HTTP 404"}]}}`}

	out, err := runCLI(t, f, "status", testNS, "-o", "table")
	require.NoError(t, err)
	assert.Contains(t, out, "Downloading FFmpeg build")
	assert.Contains(t, out, "HTTP 404")
}

// The manual documents `quiver watch <ns>` as the raw live event stream, and
// points at it from Troubleshooting for a run that looks stuck.
func TestWatch_StreamsRuntimeEvents(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: installScript()}

	out, err := runCLI(t, f, "watch", testNS, "-o", "table")
	require.NoError(t, err)
	assert.Contains(t, out, "installing")
	assert.Contains(t, out, "ready")
	assert.Contains(t, out, "Fetching binary")
}

func TestWatch_RejectsInvalidNamespace(t *testing.T) {
	f := &fakeDaemon{t: t}

	_, err := runCLI(t, f, "watch", "not-a-namespace")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid namespace")
}

// `quiver status <ns> -w` is documented as "keep watching live", so it streams
// the same events rather than returning one snapshot.
func TestStatus_WatchFlagStreams(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: installScript()}

	out, err := runCLI(t, f, "status", testNS, "-w", "-o", "table")
	require.NoError(t, err)
	assert.Contains(t, out, "installing")
	assert.Contains(t, out, "ready")
}

func TestStatus_WatchWithoutNamespaceIsUsageError(t *testing.T) {
	f := &fakeDaemon{t: t}

	_, err := runCLI(t, f, "status", "-w")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs a namespace")
}

func TestWatch_JSONPayloadCarriesEvents(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: installScript()}

	out, err := runCLI(t, f, "watch", testNS, "-o", "json")
	require.NoError(t, err)

	var w output.Watch
	require.NoError(t, json.Unmarshal([]byte(out), &w), "watch payload: %s", out)
	assert.Equal(t, testNS, w.Subject)
	require.Len(t, w.Events, 3)
	assert.Equal(t, "installing", w.Events[0].State)
	assert.Equal(t, "ready", w.Events[2].State)
}

// An arrow that has never run has neither an active run nor a last return, so
// there is no step list to show and status must not invent one.
func TestStatus_NoStepsWhenNothingHasRun(t *testing.T) {
	f := &fakeDaemon{t: t, runtimeDetail: `{"namespace":"` + testNS + `","state":"absent"}`}

	out, err := runCLI(t, f, "status", testNS, "-o", "table")
	require.NoError(t, err)
	assert.Contains(t, out, "absent")
	assert.NotContains(t, out, "STEPS")
}

func TestWatch_UnreachableDaemonIsConnectionError(t *testing.T) {
	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.Attach(root, commands.Deps{Version: "test", IsTTY: func() bool { return false }})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	cfgPath := filepath.Join(t.TempDir(), "cli.yaml")
	// Port 0 is never listening, so the dial fails without touching a daemon.
	root.SetArgs([]string{"watch", testNS, "--server", "http://127.0.0.1:0", "--config", cfgPath})

	err := root.Execute()
	require.Error(t, err)
	assert.Equal(t, client.ExitConnection, commands.ExitCode(err))
}

// ─── arrow group ─────────────────────────────────────────────────────────────

func TestArrowAdd_Posts(t *testing.T) {
	f := &fakeDaemon{t: t}

	_, err := runCLI(t, f, "arrow", "add", testNS)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/arrow/github.com%2Fuser%2Fapp")
}

func TestArrowRemove_Deletes(t *testing.T) {
	f := &fakeDaemon{t: t}

	_, err := runCLI(t, f, "arrow", "remove", testNS, "--yes")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "DELETE /v0/arrow/github.com%2Fuser%2Fapp")
}

func TestArrowList_Table(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "arrow", "list")
	require.NoError(t, err)
	assert.Contains(t, out, testNS)
}

func TestArrowShow_Detail(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "arrow", "show", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "App")
}

// ─── collection group ────────────────────────────────────────────────────────

func TestCollectionFollow_Posts(t *testing.T) {
	f := &fakeDaemon{t: t}

	_, err := runCLI(t, f, "collection", "follow", "github.com/user/col")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/collection/github.com%2Fuser%2Fcol/follow")
}

func TestCollectionUnfollow_Deletes(t *testing.T) {
	f := &fakeDaemon{t: t}

	_, err := runCLI(t, f, "collection", "unfollow", "github.com/user/col", "--yes")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "DELETE /v0/collection/github.com%2Fuser%2Fcol/follow")
}

func TestCollectionList_Table(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "collection", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "github.com/user/col")
}

func TestCollectionShow_ListsArrows(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "collection", "show", "github.com/user/col")
	require.NoError(t, err)
	assert.Contains(t, out, "Col")
	assert.Contains(t, out, testNS)
}

func TestCollectionUpdate_PostsManifest(t *testing.T) {
	f := &fakeDaemon{t: t}

	_, err := runCLI(t, f, "collection", "update", "github.com/user/col")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/collection/github.com%2Fuser%2Fcol/manifest")
}

// ─── context group ───────────────────────────────────────────────────────────

func TestContext_AddUseCurrentFlow(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")

	_, err := runCLIConfig(t, cfg, "context", "add", "homelab", "--ctx-server", "tcp://10.0.0.5:40257")
	require.NoError(t, err)

	out, err := runCLIConfig(t, cfg, "context", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "homelab")
	assert.Contains(t, out, "local")

	_, err = runCLIConfig(t, cfg, "context", "use", "homelab")
	require.NoError(t, err)

	out, err = runCLIConfig(t, cfg, "context", "current")
	require.NoError(t, err)
	assert.Contains(t, out, "homelab")
}

func TestContext_ShowAndRemove(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	_, err := runCLIConfig(t, cfg, "context", "add", "r", "--ctx-server", "tcp://a:1")
	require.NoError(t, err)

	out, err := runCLIConfig(t, cfg, "context", "show", "r")
	require.NoError(t, err)
	assert.Contains(t, out, "tcp://a:1")

	_, err = runCLIConfig(t, cfg, "context", "remove", "r")
	require.NoError(t, err)

	out, _ = runCLIConfig(t, cfg, "context", "list")
	assert.NotContains(t, out, "tcp://a:1")
}

// ─── system ──────────────────────────────────────────────────────────────────

func TestHealth_OK(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "health")
	require.NoError(t, err)
	assert.Contains(t, out, "ok")
}

func TestVersion_ShowsClientAndDaemon(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "version")
	require.NoError(t, err)
	assert.Contains(t, out, "test") // CLI version
	assert.Contains(t, out, "26.5") // daemon version
}

func TestVersion_ClientOnlySkipsDaemon(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "version", "--client-only")
	require.NoError(t, err)
	assert.Contains(t, out, "test")
	assert.NotContains(t, out, "26.5")
}
