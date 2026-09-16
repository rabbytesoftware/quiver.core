package commands_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands"
	"github.com/rabbytesoftware/quiver.core/internal/cli/testutil"
)

// runWith runs the CLI against an arbitrary handler with custom deps/stdin.
func runWith(
	t *testing.T,
	handler http.Handler,
	deps commands.Deps,
	stdin io.Reader,
	args ...string,
) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.Attach(root, deps)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if stdin != nil {
		root.SetIn(stdin)
	}

	cfgPath := filepath.Join(t.TempDir(), "cli.yaml")
	if handler != nil {
		srv := httptest.NewServer(handler)
		t.Cleanup(srv.Close)
		args = append(args, "--server", srv.URL)
	}
	root.SetArgs(append(args, "--config", cfgPath))

	err := root.Execute()
	return out.String(), err
}

func noTTY() commands.Deps {
	return commands.Deps{Version: "test", IsTTY: func() bool { return false }}
}

func failingServer() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))
	})
}

// ─── exit codes ──────────────────────────────────────────────────────────────

func TestExitCode_Mapping(t *testing.T) {
	assert.Equal(t, 0, commands.ExitCode(nil))
	assert.Equal(t, 1, commands.ExitCode(errors.New("plain")))
	assert.Equal(t, 3, commands.ExitCode(&client.ConnError{Server: "s", Err: errors.New("x")}))

	_, err := runWith(t, nil, noTTY(), nil, "frobnicate")
	require.Error(t, err)
	assert.Equal(t, 2, commands.ExitCode(err))
	assert.Contains(t, err.Error(), "frobnicate")
}

// ─── active state ────────────────────────────────────────────────────────────

func TestIsActiveState_DelegatesToRuntimePackage(t *testing.T) {
	assert.True(t, commands.IsActiveState("running"))
	assert.True(t, commands.IsActiveState("installing"))
	assert.False(t, commands.IsActiveState("absent"))
	assert.False(t, commands.IsActiveState("ready"))
}

// ─── table + yaml rendering ──────────────────────────────────────────────────

func TestTables_AllCommands(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want []string
	}{
		{"info", []string{"info", testNS, "-o", "table"}, []string{"App", "ready", "web"}},
		{"methods", []string{"methods", testNS, "-o", "table"}, []string{"backup", "seed-db", "custom"}},
		{
			"methods builtins",
			[]string{"methods", testNS, "--include-builtins", "-o", "table"},
			[]string{"install", "built-in"},
		},
		{"search", []string{"search", "github.com/user/*", "-o", "table"}, []string{testNS, "result"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runCLI(t, &fakeDaemon{t: t}, tc.args...)
			require.NoError(t, err)
			for _, want := range tc.want {
				assert.Contains(t, out, want)
			}
		})
	}
}

func TestList_TableShowsRef(t *testing.T) {
	out, err := runCLI(t, &fakeDaemon{t: t}, "list", "-o", "table")
	require.NoError(t, err)
	assert.Contains(t, out, "REF")
}

func TestList_YAMLOutput(t *testing.T) {
	out, err := runCLI(t, &fakeDaemon{t: t}, "list", "-o", "yaml")
	require.NoError(t, err)
	assert.Contains(t, out, "arrows:")
	assert.Contains(t, out, "collections:")
}

func TestList_UnknownFormatIsUsageError(t *testing.T) {
	_, err := runCLI(t, &fakeDaemon{t: t}, "list", "-o", "xml")
	require.Error(t, err)
	assert.Equal(t, 2, commands.ExitCode(err))
}

func TestDispatch_CustomMethodWithRootFlags(t *testing.T) {
	f := &fakeDaemon{t: t}
	out, err := runCLI(t, f, testNS, "backup", "--detach", "--data", "k=v")
	require.NoError(t, err)
	assert.Contains(t, out, "started, not waiting")
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/backup")
}

func TestDispatch_NoArgsShowsHelp(t *testing.T) {
	out, err := runCLI(t, &fakeDaemon{t: t})
	require.NoError(t, err)
	assert.Contains(t, out, "Usage")
}

// ─── namespace validation ────────────────────────────────────────────────────

func TestValidNS_RejectsGarbage(t *testing.T) {
	for _, args := range [][]string{
		{"info", "notanamespace"},
		{"methods", "notanamespace"},
	} {
		_, err := runCLI(t, &fakeDaemon{t: t}, args...)
		require.Error(t, err, "args: %v", args)
		assert.Equal(t, 2, commands.ExitCode(err), "args: %v", args)
	}
}

// ─── session + config errors ─────────────────────────────────────────────────

func TestSession_CorruptConfigErrors(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "cli.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("contexts: [not-a-context"), 0o600))

	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.Attach(root, noTTY())
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"list", "--config", cfgPath})
	assert.Error(t, root.Execute())
}

func TestSession_UnknownContextErrors(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	_, err := runCLIConfig(t, cfg, "list", "--context", "ghost")
	assert.Error(t, err)
}

func TestSession_BadServerSchemeErrors(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	_, err := runCLIConfig(t, cfg, "list", "--server", "ftp://nope")
	assert.Error(t, err)
}

func TestSession_DefaultConfigPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.Attach(root, noTTY())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	// list reads the default config through a.session without needing an
	// explicit --config flag. The local socket resolves but nothing is
	// listening, so the failure comes back as a connection error rather
	// than a config-loading one, which is what proves the default path
	// was found.
	root.SetArgs([]string{"list"})

	err := root.Execute()
	require.Error(t, err)
	assert.Equal(t, client.ExitConnection, commands.ExitCode(err))
}

func TestSession_EnsureDaemonCalledForUnixServers(t *testing.T) {
	called := false
	deps := noTTY()
	deps.EnsureDaemon = func(context.Context) error { called = true; return errors.New("boot failed") }

	_, err := runWith(t, nil, deps, nil,
		"list", "--server", "unix:///nonexistent/quiver.sock")
	require.Error(t, err)
	assert.True(t, called)
	assert.Contains(t, err.Error(), "boot failed")
}

// ─── API error paths ─────────────────────────────────────────────────────────

func TestCommands_DaemonErrorsPropagate(t *testing.T) {
	testCases := [][]string{
		{"list"},
		{"search", "x"},
		{"info", testNS},
		{"info", testNS, "--manifest"},
		{"methods", testNS},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := runWith(t, failingServer(), noTTY(), nil, args...)
			assert.Error(t, err)
		})
	}
}

func TestManifestFetch_StripsRef(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{"methods", []string{"methods", testNS + "@v1.0.0"}},
		{"info manifest", []string{"info", testNS + "@v1.0.0", "--manifest"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.EscapedPath()
				_, _ = w.Write([]byte(`{"success":true,"data":{"targets":{}}}`))
			})
			_, err := runWith(t, handler, noTTY(), nil, tc.args...)
			require.NoError(t, err)
			assert.NotContains(t, gotPath, "@",
				"manifest endpoints take a bare namespace, the ref must be stripped")
			assert.NotContains(t, gotPath, "%40")
		})
	}
}

func TestMethods_UnparsableManifestErrors(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":"not-an-object"}`))
	})
	_, err := runWith(t, handler, noTTY(), nil, "methods", testNS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse manifest")
}

func TestSessionErrors_AllCommands(t *testing.T) {
	testCases := [][]string{
		{"list"},
		{"search", "x"},
		{"info", testNS},
		{"methods", testNS},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "cli.yaml")
			_, err := runCLIConfig(t, cfg, append(args, "--server", "ftp://nope")...)
			assert.Error(t, err)
		})
	}
}

func TestLoadConfig_NoHomeErrors(t *testing.T) {
	testutil.RequireUnix(t)

	t.Setenv("HOME", "")

	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.Attach(root, noTTY())
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"list"})
	assert.Error(t, root.Execute())
}

func TestList_CollectionFetchFailurePropagates(t *testing.T) {
	f := &fakeDaemon{t: t}
	base := f.handler()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == "/v0/collection" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))
			return
		}
		base.ServeHTTP(w, r)
	})
	_, err := runWith(t, handler, noTTY(), nil, "list")
	assert.Error(t, err)
}
