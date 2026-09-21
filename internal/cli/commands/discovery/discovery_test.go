// Internal (package discovery, not discovery_test) so this file can exercise
// matches/globMatch directly alongside the black-box command tests — the
// brief calls for a single test file, and those two helpers have no other
// package that could test them without exporting pure string-matching
// internals for no other reason than the test.
package discovery

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
)

const testNS = "github.com/user/app"

// ─── fixtures ────────────────────────────────────────────────────────────────

// newSession builds a session.Session with an active context pointing at
// server, for a non-TTY invocation. Mirrors the pattern in
// commands/collection/collection_test.go.
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

// fakeDiscoveryDaemon serves the arrow/collection endpoints the discovery
// package's commands call.
type fakeDiscoveryDaemon struct {
	// fail makes every request return 500, for daemon-error-propagation
	// tests.
	fail bool
	// manifest overrides the arrow manifest payload; empty means the
	// default manifest with two custom methods.
	manifest string
	// collectionFails makes only GET /v0/collection fail, for the
	// partial-failure case list must still propagate.
	collectionFails bool
}

func (f *fakeDiscoveryDaemon) manifestBody() string {
	if f.manifest != "" {
		return f.manifest
	}
	return `{"namespace":"` + testNS + `","name":"App","targets":{"linux/amd64":{"lifecycle":{},` +
		`"methods":{"backup":{"available_in":["ready"],"steps":[]},` +
		`"seed-db":{"available_in":["ready"],"steps":[]}}}}}`
}

func (f *fakeDiscoveryDaemon) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.fail {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))
			return
		}

		path := r.URL.EscapedPath()

		switch {
		case path == "/v0/collection" && f.collectionFails:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))
		case path == "/v0/arrow" && r.Method == http.MethodGet:
			ok(w, `[{"namespace":"`+testNS+`","name":"App","description":"An app","tags":["web"],`+
				`"versions":[{"ref":"`+testNS+`@v1","version":"1.0.0","state":"ready",`+
				`"installed_at":"2026-01-01T00:00:00Z"}]}]`)
		case path == "/v0/arrow/github.com%2Fuser%2Fapp" && r.Method == http.MethodGet:
			ok(w, `{"namespace":"`+testNS+`","name":"App","version":"1.0.0","description":"An app",`+
				`"state":"ready","tags":["web"],"user_installed":true}`)
		case path == "/v0/arrow/github.com%2Fuser%2Fapp/manifest" && r.Method == http.MethodGet:
			ok(w, f.manifestBody())
		case path == "/v0/collection" && r.Method == http.MethodGet:
			ok(w, `[{"namespace":"github.com/user/col","name":"Col","description":"d","tags":[],`+
				`"arrow_count":2,"followed":true}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"error":"not found"}`))
		}
	})
}

func ok(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true,"data":` + data + `}`))
}

// runDiscovery runs the discovery commands (list/search/info/methods, all
// root-level) against sess, in the given output format (empty means the
// runner's default: json on a non-TTY session).
func runDiscovery(
	t *testing.T, sess session.Session, outputFormat string, args ...string,
) (string, error) {
	t.Helper()

	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(New(sess, runner.New(&runner.Flags{Output: outputFormat})).Cmd()...)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)

	err := root.Execute()

	return out.String(), err
}

func decodeCatalog(t *testing.T, out string) output.Catalog {
	t.Helper()

	var c output.Catalog
	require.NoError(t, json.Unmarshal([]byte(out), &c), "catalog payload: %s", out)

	return c
}

// ─── Cmd ─────────────────────────────────────────────────────────────────────

func TestCmd_ReturnsListSearchInfoMethods(t *testing.T) {
	got := New(newSession(t, "unix:///unused.sock"), runner.New(&runner.Flags{})).Cmd()

	require.Len(t, got, 4)
	assert.Equal(t, "list", got[0].Name())
	assert.Equal(t, "search <pattern>", got[1].Use)
	assert.Equal(t, "info <namespace>", got[2].Use)
	assert.Equal(t, "methods <namespace>", got[3].Use)
}

// ─── list / search ───────────────────────────────────────────────────────────

func TestList_TableShowsArrowsAndCollections(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	out, err := runDiscovery(t, newSession(t, srv.URL), "table", "list")
	require.NoError(t, err)
	assert.Contains(t, out, testNS)
	assert.Contains(t, out, "github.com/user/col")
	assert.Contains(t, out, "ARROWS")
	assert.Contains(t, out, "COLLECTIONS")
}

func TestList_JSONIsCombinedObject(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	out, err := runDiscovery(t, newSession(t, srv.URL), "json", "list")
	require.NoError(t, err)

	c := decodeCatalog(t, out)
	assert.Len(t, c.Arrows, 1)
	assert.Len(t, c.Collections, 1)
	assert.Equal(t, len(c.Arrows)+len(c.Collections), c.Total)
	assert.Empty(t, c.Query)
	assert.NotContains(t, out, `"query"`)
}

func TestList_FilterGlob(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	out, err := runDiscovery(t, newSession(t, srv.URL), "json", "list", "-F", "github.com/other/*")
	require.NoError(t, err)
	assert.NotContains(t, out, testNS)
}

func TestList_YAMLOutput(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	out, err := runDiscovery(t, newSession(t, srv.URL), "yaml", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "arrows:")
	assert.Contains(t, out, "collections:")
}

func TestList_UnknownFormatIsUsageError(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	_, err := runDiscovery(t, newSession(t, srv.URL), "xml", "list")
	require.Error(t, err)
}

func TestList_NoMatchesEncodesEmptyArrays(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	out, err := runDiscovery(t, newSession(t, srv.URL), "json", "search", "gitlab.com/*")
	require.NoError(t, err)

	c := decodeCatalog(t, out)
	assert.Equal(t, 0, c.Total)
	require.NotNil(t, c.Arrows)
	require.NotNil(t, c.Collections)
	assert.Contains(t, out, `"arrows": []`)
}

func TestList_ArrowFetchFailurePropagates(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{fail: true}).handler())
	defer srv.Close()

	_, err := runDiscovery(t, newSession(t, srv.URL), "", "list")
	assert.Error(t, err)
}

func TestList_CollectionFetchFailurePropagates(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{collectionFails: true}).handler())
	defer srv.Close()

	_, err := runDiscovery(t, newSession(t, srv.URL), "", "list")
	assert.Error(t, err)
}

func TestList_SessionErrorPropagates(t *testing.T) {
	_, err := runDiscovery(t, newSession(t, "ftp://nope"), "", "list")
	assert.Error(t, err)
}

func TestSearch_MatchesPattern(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	out, err := runDiscovery(t, newSession(t, srv.URL), "", "search", "github.com/user/*")
	require.NoError(t, err)
	assert.Contains(t, out, testNS)
}

func TestSearch_RecordsTheQuery(t *testing.T) {
	const pattern = "github.com/user/*"
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	out, err := runDiscovery(t, newSession(t, srv.URL), "json", "search", pattern)
	require.NoError(t, err)
	assert.Equal(t, pattern, decodeCatalog(t, out).Query)
}

func TestSearch_TableShowsCountOnlyWhenFiltered(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	unfiltered, err := runDiscovery(t, newSession(t, srv.URL), "table", "list")
	require.NoError(t, err)
	assert.NotContains(t, unfiltered, "result(s)")

	filtered, err := runDiscovery(t, newSession(t, srv.URL), "table", "search", "github.com/user/*")
	require.NoError(t, err)
	assert.Contains(t, filtered, "result(s)")
}

func TestSearch_SessionErrorPropagates(t *testing.T) {
	_, err := runDiscovery(t, newSession(t, "ftp://nope"), "", "search", "x")
	assert.Error(t, err)
}

// ─── info ────────────────────────────────────────────────────────────────────

func TestInfo_ShowsDetail(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	out, err := runDiscovery(t, newSession(t, srv.URL), "", "info", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "App")
	assert.Contains(t, out, "ready")
}

func TestInfo_ManifestFlagReturnsRaw(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	out, err := runDiscovery(t, newSession(t, srv.URL), "", "info", testNS, "--manifest")
	require.NoError(t, err)
	assert.Contains(t, out, `"targets"`)
}

func TestInfo_ManifestFetchStripsRef(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"success":true,"data":{"targets":{}}}`))
	}))
	defer srv.Close()

	_, err := runDiscovery(t, newSession(t, srv.URL), "", "info", testNS+"@v1.0.0", "--manifest")
	require.NoError(t, err)
	assert.NotContains(t, gotPath, "@")
	assert.NotContains(t, gotPath, "%40")
}

func TestInfo_InvalidNamespaceIsUsageError(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	_, err := runDiscovery(t, newSession(t, srv.URL), "", "info", "notanamespace")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid namespace")
}

func TestInfo_DaemonErrorPropagates(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{fail: true}).handler())
	defer srv.Close()

	_, err := runDiscovery(t, newSession(t, srv.URL), "", "info", testNS)
	assert.Error(t, err)
}

func TestInfo_ManifestDaemonErrorPropagates(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{fail: true}).handler())
	defer srv.Close()

	_, err := runDiscovery(t, newSession(t, srv.URL), "", "info", testNS, "--manifest")
	assert.Error(t, err)
}

func TestInfo_SessionErrorPropagates(t *testing.T) {
	_, err := runDiscovery(t, newSession(t, "ftp://nope"), "", "info", testNS)
	assert.Error(t, err)
}

func TestInfo_ManifestSessionErrorPropagates(t *testing.T) {
	_, err := runDiscovery(t, newSession(t, "ftp://nope"), "", "info", testNS, "--manifest")
	assert.Error(t, err)
}

// ─── methods ─────────────────────────────────────────────────────────────────

func TestMethods_ListsCustomMethods(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	out, err := runDiscovery(t, newSession(t, srv.URL), "", "methods", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "backup")
	assert.Contains(t, out, "seed-db")
	assert.NotContains(t, out, "install ·")
}

func TestMethods_IncludeBuiltins(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	out, err := runDiscovery(t, newSession(t, srv.URL), "", "methods", testNS, "--include-builtins")
	require.NoError(t, err)
	assert.Contains(t, out, "install")
	assert.Contains(t, out, "backup")
}

func TestMethods_TableRendersKindColumn(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	out, err := runDiscovery(t, newSession(t, srv.URL), "table", "methods", testNS, "--include-builtins")
	require.NoError(t, err)
	assert.Contains(t, out, "built-in")
	assert.Contains(t, out, "custom")
}

func TestMethods_InvalidNamespaceIsUsageError(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{}).handler())
	defer srv.Close()

	_, err := runDiscovery(t, newSession(t, srv.URL), "", "methods", "notanamespace")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid namespace")
}

func TestMethods_DaemonErrorPropagates(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{fail: true}).handler())
	defer srv.Close()

	_, err := runDiscovery(t, newSession(t, srv.URL), "", "methods", testNS)
	assert.Error(t, err)
}

func TestMethods_SessionErrorPropagates(t *testing.T) {
	_, err := runDiscovery(t, newSession(t, "ftp://nope"), "", "methods", testNS)
	assert.Error(t, err)
}

func TestMethods_UnparsableManifestErrors(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{manifest: `"not-an-object"`}).handler())
	defer srv.Close()

	_, err := runDiscovery(t, newSession(t, srv.URL), "", "methods", testNS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse manifest")
}

func TestMethods_EmptyManifestHasNoCustomMethods(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{manifest: `{"targets":{}}`}).handler())
	defer srv.Close()

	out, err := runDiscovery(t, newSession(t, srv.URL), "json", "methods", testNS)
	require.NoError(t, err)
	assert.Equal(t, "[]\n", out)
}

func TestMethods_EmptyManifestWithBuiltinsListsFiveMethods(t *testing.T) {
	srv := httptest.NewServer((&fakeDiscoveryDaemon{manifest: `{"targets":{}}`}).handler())
	defer srv.Close()

	out, err := runDiscovery(t, newSession(t, srv.URL), "json", "methods", testNS, "--include-builtins")
	require.NoError(t, err)

	var methods []methodInfo
	require.NoError(t, json.Unmarshal([]byte(out), &methods))
	assert.Len(t, methods, 5)
}

// ─── matches / globMatch ─────────────────────────────────────────────────────

func TestMatches_GlobCrossesSlash(t *testing.T) {
	ns := "github.com/user/quiver.arrow-discord"

	testCases := []struct {
		name    string
		pattern string
		want    bool
	}{
		{"match all", "*", true},
		{"surrounding wildcards", "*discord*", true},
		{"leading wildcard across slashes", "*/quiver.arrow-discord", true},
		{"trailing wildcard on name", "discord*", true},
		{"exact namespace", ns, true},
		{"single-segment wildcard", "github.com/user/*", true},
		{"substring fallback", "quiver.arrow", true},
		{"empty matches all", "", true},
		{"no match", "nginx", false},
		{"question mark wildcard", "discor?", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := matches(tc.pattern, ns, "discord")
			assert.Equal(t, tc.want, got, "matches(%q, %q, %q)", tc.pattern, ns, "discord")
		})
	}
}

func TestMatches_NameOnlyMatchDoesNotRequireNamespaceMatch(t *testing.T) {
	assert.True(t, matches("discord*", "github.com/user/app", "discord-bot"))
}
