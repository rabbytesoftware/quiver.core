package arrow_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

const testNS = "github.com/user/app"

// ─── fixtures ────────────────────────────────────────────────────────────────

// newSession builds a session.Session pointed at server, for a non-TTY
// invocation. Mirrors the pattern in commands/session/session_test.go.
func newSession(t *testing.T, server string) session.Session {
	t.Helper()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cli.yaml")
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.NoError(t, cfg.Add(config.Context{Name: "test", Server: server}, true))

	return session.New(
		session.Deps{IsTTYFunc: func() bool { return false }},
		&session.Flags{Config: cfgPath},
	)
}

func newTestTheme(t *testing.T) theme.Theme {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "0")

	var buf bytes.Buffer

	return theme.New(lipgloss.NewRenderer(&buf))
}

// fakeArrowDaemon serves the arrow-only endpoints the arrow package's
// commands call.
type fakeArrowDaemon struct {
	// mutationStatus overrides the status returned for POST/PATCH/DELETE
	// mutations; zero means the default 202 Accepted.
	mutationStatus int
	// fail makes every request return 500, for daemon-error-propagation
	// tests.
	fail bool

	mu    sync.Mutex
	posts []string // recorded "METHOD path" of mutations
}

func (f *fakeArrowDaemon) record(r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posts = append(f.posts, r.Method+" "+r.URL.Path)
}

func (f *fakeArrowDaemon) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.posts...)
}

func arrowOK(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true,"data":` + data + `}`))
}

func (f *fakeArrowDaemon) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.fail {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))

			return
		}

		path := r.URL.Path

		switch {
		case path == "/v0/arrow" && r.Method == http.MethodGet:
			arrowOK(w, `[{"namespace":"`+testNS+`","name":"App","description":"An app",`+
				`"tags":["web"],"versions":[{"ref":"`+testNS+`@v1","state":"ready",`+
				`"installed_at":"2026-01-01T00:00:00Z"}]}]`)
		case path == "/v0/arrow/"+testNS && r.Method == http.MethodGet:
			arrowOK(w, `{"namespace":"`+testNS+`","name":"App","description":"An app",`+
				`"state":"ready","license":"MIT","tags":["web"],`+
				`"installed_constraint":">=1.0.0","installed_at":"2026-01-01T00:00:00Z",`+
				`"user_installed":true}`)
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

// runArrow runs the arrow command tree against f, in the given output format
// (empty means the runner's default: json on a non-TTY session).
func runArrow(t *testing.T, f *fakeArrowDaemon, outputFormat string, args ...string) (string, error) {
	t.Helper()

	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	cmds := arrow.New(newSession(t, srv.URL), runner.New(&runner.Flags{Output: outputFormat}))
	root := cmds.Cmd()

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)

	err := root.Execute()

	return out.String(), err
}

// assertMutation decodes out as a Mutation payload and checks it describes
// the operation the command was asked to perform.
func assertMutation(t *testing.T, out, action, subject string) {
	t.Helper()

	var m output.Mutation
	require.NoError(t, json.Unmarshal([]byte(out), &m), "mutation payload: %s", out)

	assert.Equal(t, output.Action(action), m.Action)
	assert.Equal(t, subject, m.Subject)

	_, err := time.Parse(time.RFC3339, m.At)
	assert.NoError(t, err, "at must be RFC3339, got %q", m.At)
}

// ─── add / remove / refresh ──────────────────────────────────────────────────

func TestArrowAdd_Posts(t *testing.T) {
	f := &fakeArrowDaemon{}

	_, err := runArrow(t, f, "", "add", testNS)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/arrow/"+testNS)
}

func TestArrowRemove_Deletes(t *testing.T) {
	f := &fakeArrowDaemon{}

	_, err := runArrow(t, f, "", "remove", testNS, "--yes")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "DELETE /v0/arrow/"+testNS)
}

func TestArrowRemove_YesShorthandSkipsConfirmation(t *testing.T) {
	f := &fakeArrowDaemon{}

	out, err := runArrow(t, f, "", "remove", testNS, "-y")
	require.NoError(t, err)
	assertMutation(t, out, "remove", testNS)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "DELETE /v0/arrow/"+testNS)
}

func TestArrowRemove_NonTTYWithoutYes_Refuses(t *testing.T) {
	f := &fakeArrowDaemon{}

	_, err := runArrow(t, f, "", "remove", testNS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
	assert.Empty(t, f.recorded(), "a refused confirmation must not reach the daemon")
}

func TestArrowRefresh_PatchesManifest(t *testing.T) {
	f := &fakeArrowDaemon{}

	out, err := runArrow(t, f, "", "refresh", testNS)
	require.NoError(t, err)
	assertMutation(t, out, "refresh", testNS)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "PATCH /v0/arrow/"+testNS)
}

// ─── list ────────────────────────────────────────────────────────────────────

func TestArrowList_Table(t *testing.T) {
	f := &fakeArrowDaemon{}

	out, err := runArrow(t, f, "", "list")
	require.NoError(t, err)
	assert.Contains(t, out, testNS)
}

func TestArrowList_TableShowsRef(t *testing.T) {
	f := &fakeArrowDaemon{}

	out, err := runArrow(t, f, "table", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "REF")
	assert.Contains(t, out, testNS+"@v1", "the registered ref is the removal handle and must be visible")
}

func TestArrowList_DaemonError_Propagates(t *testing.T) {
	f := &fakeArrowDaemon{fail: true}

	_, err := runArrow(t, f, "", "list")
	assert.Error(t, err)
}

// ─── show ────────────────────────────────────────────────────────────────────

func TestArrowShow_Detail(t *testing.T) {
	f := &fakeArrowDaemon{}

	out, err := runArrow(t, f, "", "show", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "App")
}

func TestArrowShow_DaemonError_Propagates(t *testing.T) {
	f := &fakeArrowDaemon{fail: true}

	_, err := runArrow(t, f, "", "show", testNS)
	assert.Error(t, err)
}

// ─── mutation payload shapes ─────────────────────────────────────────────────

// Piped stdout defaults to json, the same rule every other command follows.
func TestArrowMutations_PipedEmitStructuredPayload(t *testing.T) {
	testCases := []struct {
		name   string
		args   []string
		action string
	}{
		{"add", []string{"add", testNS}, "add"},
		{"remove", []string{"remove", testNS, "-y"}, "remove"},
		{"refresh", []string{"refresh", testNS}, "refresh"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runArrow(t, &fakeArrowDaemon{}, "", tc.args...)
			require.NoError(t, err)

			assertMutation(t, out, tc.action, testNS)
		})
	}
}

// -o yaml must describe the same mutation as -o json.
func TestArrowMutations_YAMLMatchesJSON(t *testing.T) {
	out, err := runArrow(t, &fakeArrowDaemon{}, "yaml", "add", testNS)
	require.NoError(t, err)

	assert.Contains(t, out, "action: add")
	assert.Contains(t, out, "subject: "+testNS)
	assert.NotContains(t, out, "actionn")
}

// The table path prints one human-readable line.
func TestArrowMutations_TableRendersOneLine(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want string
	}{
		{"add", []string{"add", testNS}, "added " + testNS},
		{"remove", []string{"remove", testNS, "-y"}, "removed " + testNS},
		{"refresh", []string{"refresh", testNS}, "refreshed " + testNS},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runArrow(t, &fakeArrowDaemon{}, "table", tc.args...)
			require.NoError(t, err)

			assert.Contains(t, out, tc.want)
			assert.Equal(t, 1, strings.Count(strings.TrimSpace(out), "\n")+1,
				"a mutation renders one line, got %q", out)
		})
	}
}

// A failed mutation must not render a payload: the error is the result, and a
// payload on stdout would tell a script the opposite.
func TestArrowMutation_Failure_WritesNoPayload(t *testing.T) {
	f := &fakeArrowDaemon{mutationStatus: 500}

	out, err := runArrow(t, f, "", "add", testNS)
	require.Error(t, err)

	assert.NotContains(t, out, `"action"`)
}

// ─── namespace validation ────────────────────────────────────────────────────

func TestArrowCommands_InvalidNamespace_ReturnsUsageError(t *testing.T) {
	testCases := [][]string{
		{"add", "notanamespace"},
		{"remove", "notanamespace"},
		{"refresh", "notanamespace"},
		{"show", "notanamespace"},
		{"seed", "notanamespace"},
	}

	for _, args := range testCases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := runArrow(t, &fakeArrowDaemon{}, "", args...)
			require.Error(t, err, "args: %v", args)
			assert.Contains(t, err.Error(), "invalid namespace")
		})
	}
}

// ─── seed ────────────────────────────────────────────────────────────────────

func TestSeedCmd_FileFlag_ReadsAndSends(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/arrow/github.com/u/r/manifest", r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte("name: test\n"), 0o600))

	cmds := arrow.New(newSession(t, srv.URL), runner.New(&runner.Flags{Output: "json"}))
	root := cmds.Cmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"seed", "github.com/u/r", "--file", manifestPath})
	require.NoError(t, root.Execute())
	assert.Equal(t, "name: test\n", gotBody)
}

func TestSeedCmd_Stdin_ReadsAndSends(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		assert.Equal(t, "name: piped\n", string(b))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	cmds := arrow.New(newSession(t, srv.URL), runner.New(&runner.Flags{Output: "json"}))
	root := cmds.Cmd()
	root.SetOut(&bytes.Buffer{})
	root.SetIn(strings.NewReader("name: piped\n"))
	root.SetArgs([]string{"seed", "github.com/u/r", "--file", "-"})
	require.NoError(t, root.Execute())
}

func TestSeedCmd_NoFileFlag_ReturnsUsageError(t *testing.T) {
	cmds := arrow.New(newSession(t, "unix:///unused.sock"), runner.New(&runner.Flags{}))
	root := cmds.Cmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"seed", "github.com/u/r"})
	err := root.Execute()
	require.Error(t, err)
}

func TestSeedCmd_MissingFile_ReturnsReadError(t *testing.T) {
	cmds := arrow.New(newSession(t, "unix:///unused.sock"), runner.New(&runner.Flags{}))
	root := cmds.Cmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"seed", "github.com/u/r", "--file", filepath.Join(t.TempDir(), "missing.yaml")})
	err := root.Execute()
	require.Error(t, err)
}

// ─── exported view helpers (reused by commands/discovery) ───────────────────

func TestInstalledRefAndState_Table(t *testing.T) {
	testCases := []struct {
		name      string
		item      apidto.ArrowListItemDTO
		wantRef   string
		wantState string
	}{
		{
			name:      "no versions is absent",
			item:      apidto.ArrowListItemDTO{Namespace: testNS},
			wantRef:   "-",
			wantState: "absent",
		},
		{
			name: "installed version carries ref and state",
			item: apidto.ArrowListItemDTO{
				Versions: []apidto.InstalledVersionItemDTO{{Ref: testNS + "@v1", State: "ready"}},
			},
			wantRef:   testNS + "@v1",
			wantState: "ready",
		},
		{
			name: "empty ref on an installed version renders as a dash",
			item: apidto.ArrowListItemDTO{
				Versions: []apidto.InstalledVersionItemDTO{{Ref: "", State: "installing"}},
			},
			wantRef:   "-",
			wantState: "installing",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ref, state := arrow.InstalledRefAndState(tc.item)
			assert.Equal(t, tc.wantRef, ref)
			assert.Equal(t, tc.wantState, state)
		})
	}
}

func TestRowsFrom_MapsNamespaceNameRefState(t *testing.T) {
	in := []apidto.ArrowListItemDTO{
		{
			Namespace: testNS, Name: "App",
			Versions: []apidto.InstalledVersionItemDTO{{Ref: testNS + "@v1", State: "ready"}},
		},
	}

	out := arrow.RowsFrom(in)
	require.Len(t, out, 1)
	assert.Equal(t, testNS, out[0].Namespace)
	assert.Equal(t, "App", out[0].Name)
	assert.Equal(t, testNS+"@v1", out[0].Ref)
	assert.Equal(t, "ready", out[0].State)
}

func TestTable_RendersRowsAndEmptyState(t *testing.T) {
	th := newTestTheme(t)

	got := arrow.Table([]output.ArrowRow{
		{Namespace: testNS, Name: "App", Ref: testNS + "@v1", State: "ready"},
	}, th)
	assert.Contains(t, got, "NAMESPACE")
	assert.Contains(t, got, testNS)

	empty := arrow.Table(nil, th)
	assert.Contains(t, empty, "no arrows")
}

func TestViewDetail_OmitsEmptyOptionalFields(t *testing.T) {
	th := newTestTheme(t)

	got := arrow.ViewDetail(apidto.ArrowDetailDTO{Namespace: testNS, Name: "App", State: "ready"}, th)
	assert.Contains(t, got, "Namespace")
	assert.Contains(t, got, "Name")
	assert.NotContains(t, got, "Description")
	assert.NotContains(t, got, "License")
	assert.NotContains(t, got, "Tags")
	assert.NotContains(t, got, "Constraint")
	assert.NotContains(t, got, "Installed")
}

func TestViewDetail_IncludesAllPopulatedFields(t *testing.T) {
	th := newTestTheme(t)

	got := arrow.ViewDetail(apidto.ArrowDetailDTO{
		Namespace: testNS, Name: "App", State: "ready",
		Description:         "An app",
		License:             "MIT",
		Tags:                []string{"web", "cli"},
		InstalledConstraint: ">=1.0.0",
		InstalledAt:         "2026-01-01T00:00:00Z",
	}, th)
	assert.Contains(t, got, "Description")
	assert.Contains(t, got, "An app")
	assert.Contains(t, got, "License")
	assert.Contains(t, got, "MIT")
	assert.Contains(t, got, "Tags")
	assert.Contains(t, got, "web, cli")
	assert.Contains(t, got, "Constraint")
	assert.Contains(t, got, "Installed")
}
