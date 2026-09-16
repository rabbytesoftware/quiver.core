package collection_test

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
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/collection"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

const testNS = "github.com/user/col"

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

// fakeCollectionDaemon serves the collection-only endpoints the collection
// package's commands call.
type fakeCollectionDaemon struct {
	// mutationStatus overrides the status returned for POST/PATCH/DELETE
	// mutations; zero means the default 202 Accepted.
	mutationStatus int
	// fail makes every request return 500, for daemon-error-propagation
	// tests.
	fail bool

	mu    sync.Mutex
	posts []string // recorded "METHOD path" of mutations
}

func (f *fakeCollectionDaemon) record(r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posts = append(f.posts, r.Method+" "+r.URL.Path)
}

func (f *fakeCollectionDaemon) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.posts...)
}

func collectionOK(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true,"data":` + data + `}`))
}

func (f *fakeCollectionDaemon) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.fail {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))

			return
		}

		path := r.URL.Path

		switch {
		case path == "/v0/collection" && r.Method == http.MethodGet:
			collectionOK(w, `[{"namespace":"`+testNS+`","name":"Col","description":"d",`+
				`"tags":[],"arrow_count":2,"followed":true}]`)
		case path == "/v0/collection/"+testNS && r.Method == http.MethodGet:
			collectionOK(w, `{"namespace":"`+testNS+`","name":"Col","description":"d",`+
				`"maintainers":[],"tags":[],"arrows":[{"namespace":"github.com/user/app",`+
				`"name":"App","resolved":true}],"followed":true}`)
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

// runCollection runs the collection command tree against f, in the given
// output format (empty means the runner's default: json on a non-TTY
// session).
func runCollection(
	t *testing.T, f *fakeCollectionDaemon, outputFormat string, args ...string,
) (string, error) {
	t.Helper()

	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	cmds := collection.New(newSession(t, srv.URL), runner.New(&runner.Flags{Output: outputFormat}))
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

// ─── follow / unfollow / update ──────────────────────────────────────────────

func TestCollectionFollow_Posts(t *testing.T) {
	f := &fakeCollectionDaemon{}

	_, err := runCollection(t, f, "", "follow", testNS)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/collection/"+testNS+"/follow")
}

func TestCollectionUnfollow_Deletes(t *testing.T) {
	f := &fakeCollectionDaemon{}

	_, err := runCollection(t, f, "", "unfollow", testNS, "--yes")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "DELETE /v0/collection/"+testNS+"/follow")
}

func TestCollectionUnfollow_YesShorthandSkipsConfirmation(t *testing.T) {
	f := &fakeCollectionDaemon{}

	out, err := runCollection(t, f, "", "unfollow", testNS, "-y")
	require.NoError(t, err)
	assertMutation(t, out, "unfollow", testNS)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "DELETE /v0/collection/"+testNS+"/follow")
}

func TestCollectionUnfollow_NonTTYWithoutYes_Refuses(t *testing.T) {
	f := &fakeCollectionDaemon{}

	_, err := runCollection(t, f, "", "unfollow", testNS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
	assert.Empty(t, f.recorded(), "a refused confirmation must not reach the daemon")
}

func TestCollectionUpdate_PostsManifest(t *testing.T) {
	f := &fakeCollectionDaemon{}

	out, err := runCollection(t, f, "", "update", testNS)
	require.NoError(t, err)
	assertMutation(t, out, "update", testNS)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/collection/"+testNS+"/manifest")
}

// ─── list ────────────────────────────────────────────────────────────────────

func TestCollectionList_Table(t *testing.T) {
	f := &fakeCollectionDaemon{}

	out, err := runCollection(t, f, "", "list")
	require.NoError(t, err)
	assert.Contains(t, out, testNS)
}

func TestCollectionList_TableShowsArrowCount(t *testing.T) {
	f := &fakeCollectionDaemon{}

	out, err := runCollection(t, f, "table", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "ARROWS")
	assert.Contains(t, out, "2")
}

func TestCollectionList_DaemonError_Propagates(t *testing.T) {
	f := &fakeCollectionDaemon{fail: true}

	_, err := runCollection(t, f, "", "list")
	assert.Error(t, err)
}

// ─── show ────────────────────────────────────────────────────────────────────

func TestCollectionShow_ListsArrows(t *testing.T) {
	f := &fakeCollectionDaemon{}

	out, err := runCollection(t, f, "", "show", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "Col")
	assert.Contains(t, out, "github.com/user/app")
}

func TestCollectionShow_DaemonError_Propagates(t *testing.T) {
	f := &fakeCollectionDaemon{fail: true}

	_, err := runCollection(t, f, "", "show", testNS)
	assert.Error(t, err)
}

// ─── mutation payload shapes ─────────────────────────────────────────────────

// Piped stdout defaults to json, the same rule every other command follows.
func TestCollectionMutations_PipedEmitStructuredPayload(t *testing.T) {
	testCases := []struct {
		name   string
		args   []string
		action string
	}{
		{"follow", []string{"follow", testNS}, "follow"},
		{"unfollow", []string{"unfollow", testNS, "-y"}, "unfollow"},
		{"update", []string{"update", testNS}, "update"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runCollection(t, &fakeCollectionDaemon{}, "", tc.args...)
			require.NoError(t, err)

			assertMutation(t, out, tc.action, testNS)
		})
	}
}

// -o yaml must describe the same mutation as -o json.
func TestCollectionMutations_YAMLMatchesJSON(t *testing.T) {
	out, err := runCollection(t, &fakeCollectionDaemon{}, "yaml", "follow", testNS)
	require.NoError(t, err)

	assert.Contains(t, out, "action: follow")
	assert.Contains(t, out, "subject: "+testNS)
}

// The table path prints one human-readable line.
func TestCollectionMutations_TableRendersOneLine(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want string
	}{
		{"follow", []string{"follow", testNS}, "followed " + testNS},
		{"unfollow", []string{"unfollow", testNS, "-y"}, "unfollowed " + testNS},
		{"update", []string{"update", testNS}, "updated " + testNS},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runCollection(t, &fakeCollectionDaemon{}, "table", tc.args...)
			require.NoError(t, err)

			assert.Contains(t, out, tc.want)
			assert.Equal(t, 1, strings.Count(strings.TrimSpace(out), "\n")+1,
				"a mutation renders one line, got %q", out)
		})
	}
}

// A failed mutation must not render a payload: the error is the result, and a
// payload on stdout would tell a script the opposite.
func TestCollectionMutation_Failure_WritesNoPayload(t *testing.T) {
	f := &fakeCollectionDaemon{mutationStatus: 500}

	out, err := runCollection(t, f, "", "follow", testNS)
	require.Error(t, err)

	assert.NotContains(t, out, `"action"`)
}

// ─── namespace validation ────────────────────────────────────────────────────

func TestCollectionCommands_InvalidNamespace_ReturnsUsageError(t *testing.T) {
	testCases := [][]string{
		{"follow", "notanamespace"},
		{"unfollow", "notanamespace"},
		{"update", "notanamespace"},
		{"show", "notanamespace"},
		{"seed", "notanamespace"},
	}

	for _, args := range testCases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := runCollection(t, &fakeCollectionDaemon{}, "", args...)
			require.Error(t, err, "args: %v", args)
			assert.Contains(t, err.Error(), "invalid namespace")
		})
	}
}

// ─── seed ────────────────────────────────────────────────────────────────────

func TestSeedCmd_FileFlag_ReadsAndSends(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/collection/github.com/u/r/manifest", r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte("name: test\n"), 0o600))

	cmds := collection.New(newSession(t, srv.URL), runner.New(&runner.Flags{Output: "json"}))
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

	cmds := collection.New(newSession(t, srv.URL), runner.New(&runner.Flags{Output: "json"}))
	root := cmds.Cmd()
	root.SetOut(&bytes.Buffer{})
	root.SetIn(strings.NewReader("name: piped\n"))
	root.SetArgs([]string{"seed", "github.com/u/r", "--file", "-"})
	require.NoError(t, root.Execute())
}

func TestSeedCmd_NoFileFlag_ReturnsUsageError(t *testing.T) {
	cmds := collection.New(newSession(t, "unix:///unused.sock"), runner.New(&runner.Flags{}))
	root := cmds.Cmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"seed", "github.com/u/r"})
	err := root.Execute()
	require.Error(t, err)
}

func TestSeedCmd_MissingFile_ReturnsReadError(t *testing.T) {
	cmds := collection.New(newSession(t, "unix:///unused.sock"), runner.New(&runner.Flags{}))
	root := cmds.Cmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"seed", "github.com/u/r", "--file", filepath.Join(t.TempDir(), "missing.yaml")})
	err := root.Execute()
	require.Error(t, err)
}

// ─── exported view helpers (reused by commands/discovery) ───────────────────

func TestRowsFrom_MapsNamespaceNameArrowCount(t *testing.T) {
	in := []apidto.CollectionListItemDTO{
		{Namespace: testNS, Name: "Col", ArrowCount: 3},
	}

	out := collection.RowsFrom(in)
	require.Len(t, out, 1)
	assert.Equal(t, testNS, out[0].Namespace)
	assert.Equal(t, "Col", out[0].Name)
	assert.Equal(t, 3, out[0].Arrows)
}

func TestTable_RendersRowsAndEmptyState(t *testing.T) {
	th := newTestTheme(t)

	got := collection.Table([]output.CollectionRow{
		{Namespace: testNS, Name: "Col", Arrows: 2},
	}, th)
	assert.Contains(t, got, "NAMESPACE")
	assert.Contains(t, got, testNS)
	assert.Contains(t, got, "2")

	empty := collection.Table(nil, th)
	assert.Contains(t, empty, "no collections")
}

func TestViewDetail_OmitsEmptyOptionalFieldsAndListsNoArrows(t *testing.T) {
	th := newTestTheme(t)

	got := collection.ViewDetail(apidto.CollectionDetailDTO{Namespace: testNS, Name: "Col"}, th)
	assert.Contains(t, got, "Namespace")
	assert.Contains(t, got, "Name")
	assert.NotContains(t, got, "Description")
	assert.Contains(t, got, "no arrows")
}

func TestViewDetail_IncludesArrowsWithResolvedState(t *testing.T) {
	th := newTestTheme(t)

	got := collection.ViewDetail(apidto.CollectionDetailDTO{
		Namespace:   testNS,
		Name:        "Col",
		Description: "a collection",
		Arrows: []apidto.CollectionArrowDTO{
			{Namespace: "github.com/user/app", Name: "App", Resolved: true},
			{Namespace: "github.com/user/other", Name: "Other", Resolved: false},
		},
	}, th)
	assert.Contains(t, got, "Description")
	assert.Contains(t, got, "a collection")
	assert.Contains(t, got, "github.com/user/app")
	assert.Contains(t, got, "yes")
	assert.Contains(t, got, "github.com/user/other")
	assert.Contains(t, got, "no")
}
