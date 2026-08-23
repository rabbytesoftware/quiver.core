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

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
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

func withTTY() commands.Deps {
	return commands.Deps{Version: "test", IsTTY: func() bool { return true }}
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

// ─── table + yaml rendering ──────────────────────────────────────────────────

func TestTables_AllCommands(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want []string
	}{
		{"ps", []string{"ps", "-o", "table"}, []string{testNS, "running", "42"}},
		{"ps all", []string{"ps", "--all", "-o", "table"}, []string{"github.com/user/idle"}},
		{"status all", []string{"status", "-o", "table"}, []string{testNS, "github.com/user/idle"}},
		{"status one", []string{"status", testNS, "-o", "table"}, []string{"running", "42", "_execute"}},
		{"info", []string{"info", testNS, "-o", "table"}, []string{"App", "ready", "web"}},
		{"methods", []string{"methods", testNS, "-o", "table"}, []string{"backup", "seed-db", "custom"}},
		{
			"methods builtins",
			[]string{"methods", testNS, "--include-builtins", "-o", "table"},
			[]string{"install", "built-in"},
		},
		{"search", []string{"search", "github.com/user/*", "-o", "table"}, []string{testNS, "result"}},
		{"arrow list", []string{"arrow", "list", "-o", "table"}, []string{testNS, "App"}},
		{"arrow show", []string{"arrow", "show", testNS, "-o", "table"}, []string{"App", "ready"}},
		{"collection list", []string{"collection", "list", "-o", "table"}, []string{"github.com/user/col"}},
		{
			"collection show",
			[]string{"collection", "show", "github.com/user/col", "-o", "table"},
			[]string{"Col", testNS},
		},
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

func TestArrowList_TableShowsRef(t *testing.T) {
	out, err := runCLI(t, &fakeDaemon{t: t}, "arrow", "list", "-o", "table")
	require.NoError(t, err)
	assert.Contains(t, out, "REF")
	assert.Contains(t, out, testNS+"@v1", "the registered ref is the removal handle and must be visible")
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

func TestContextList_TableMarksActive(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	out, err := runCLIConfig(t, cfg, "context", "list", "-o", "table")
	require.NoError(t, err)
	assert.Contains(t, out, "local")
	assert.Contains(t, out, "*")
}

func TestContextShow_Table(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	_, err := runCLIConfig(t, cfg, "context", "add", "r", "--ctx-server", "tcp://a:1")
	require.NoError(t, err)

	out, err := runCLIConfig(t, cfg, "context", "show", "r", "-o", "table")
	require.NoError(t, err)
	assert.Contains(t, out, "tcp://a:1")
}

// ─── confirm gate ────────────────────────────────────────────────────────────

func TestConfirm_NonTTYWithoutForceRefuses(t *testing.T) {
	_, err := runCLI(t, &fakeDaemon{t: t}, "arrow", "remove", testNS)
	require.Error(t, err)
	assert.Equal(t, 2, commands.ExitCode(err))
	assert.Contains(t, err.Error(), "--yes")
}

func TestConfirm_TTYAcceptsYes(t *testing.T) {
	f := &fakeDaemon{t: t}
	out, err := runWith(t, f.handler(), withTTY(), strings.NewReader("y\n"),
		"arrow", "remove", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "removed")
}

func TestConfirm_TTYRejectsNo(t *testing.T) {
	f := &fakeDaemon{t: t}
	_, err := runWith(t, f.handler(), withTTY(), strings.NewReader("n\n"),
		"arrow", "remove", testNS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
}

// ─── data flag ───────────────────────────────────────────────────────────────

func TestData_ParsedIntoVariables(t *testing.T) {
	f := &fakeDaemon{t: t}
	_, err := runCLI(t, f, "install", testNS, "--detach", "--data", "port=8080")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "install")
}

func TestData_InvalidPairIsUsageError(t *testing.T) {
	_, err := runCLI(t, &fakeDaemon{t: t}, "install", testNS, "--detach", "--data", "noequals")
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
		{"install", "notanamespace"},
		{"info", "notanamespace"},
		{"methods", "notanamespace"},
		{"status", "notanamespace"},
		{"arrow", "add", "notanamespace"},
		{"arrow", "show", "notanamespace"},
		{"collection", "follow", "notanamespace"},
		{"collection", "show", "notanamespace"},
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
	root.SetArgs([]string{"health", "--config", cfgPath})
	assert.Error(t, root.Execute())
}

func TestSession_UnknownContextErrors(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	_, err := runCLIConfig(t, cfg, "health", "--context", "ghost")
	assert.Error(t, err)
}

func TestSession_BadServerSchemeErrors(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	_, err := runCLIConfig(t, cfg, "health", "--server", "ftp://nope")
	assert.Error(t, err)
}

func TestSession_DefaultConfigPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.Attach(root, noTTY())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"context", "current"})

	require.NoError(t, root.Execute())
	assert.Contains(t, out.String(), "local")
}

func TestSession_EnsureDaemonCalledForUnixServers(t *testing.T) {
	called := false
	deps := noTTY()
	deps.EnsureDaemon = func(context.Context) error { called = true; return errors.New("boot failed") }

	_, err := runWith(t, nil, deps, nil,
		"health", "--server", "unix:///nonexistent/quiver.sock")
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
		{"ps"},
		{"status"},
		{"status", testNS},
		{"arrow", "add", testNS},
		{"arrow", "remove", testNS, "--yes"},
		{"arrow", "list"},
		{"arrow", "show", testNS},
		{"collection", "follow", "github.com/user/col"},
		{"collection", "unfollow", "github.com/user/col", "--yes"},
		{"collection", "update", "github.com/user/col"},
		{"collection", "list"},
		{"collection", "show", "github.com/user/col"},
		{"health"},
		{"install", testNS, "--detach"},
		{"install", testNS},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := runWith(t, failingServer(), noTTY(), nil, args...)
			assert.Error(t, err)
		})
	}
}

func TestVersion_DaemonUnreachableStillSucceeds(t *testing.T) {
	out, err := runWith(t, failingServer(), noTTY(), nil, "version")
	require.NoError(t, err)
	assert.Contains(t, out, "client test")
	assert.Contains(t, out, "unreachable")
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

func TestInstall_PostFailsAfterSubscribe(t *testing.T) {
	f := &fakeDaemon{t: t}
	base := f.handler()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))
			return
		}
		base.ServeHTTP(w, r)
	})
	_, err := runWith(t, handler, noTTY(), nil, "install", testNS)
	assert.Error(t, err)
}

func TestInstall_StreamClosesWithoutTerminalEvent(t *testing.T) {
	f := &fakeDaemon{t: t} // empty wsScript: socket closes after 200ms
	_, err := runCLI(t, f, "install", testNS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stream closed")
}

// ─── TTY lifecycle rendering ─────────────────────────────────────────────────

func TestInstall_TTYRendersModel(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: installScript()}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	out, err := runWith(t, f.handler(), withTTY(), strings.NewReader(""),
		"install", testNS)
	require.NoError(t, err)

	// The step and the outcome, and no echo of the command the user just
	// typed — the old view opened with a "quiver install" banner.
	assert.Contains(t, out, "Fetching binary")
	assert.Contains(t, out, "install "+testNS)
	assert.NotContains(t, out, "▸")
}

func TestInstall_TTYFailureReturnsError(t *testing.T) {
	msg := "boom"
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "absent", LastReturn: &apidto.ReturnDTO{
			Method: "_install", Outcome: "failed",
			Steps: []apidto.StepProgressDTO{{Index: 0, Status: "failed", Error: &msg}},
		}},
	}}
	_, err := runWith(t, f.handler(), withTTY(), strings.NewReader(""),
		"install", testNS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed")
}

func TestSessionErrors_AllCommands(t *testing.T) {
	testCases := [][]string{
		{"list"},
		{"search", "x"},
		{"info", testNS},
		{"methods", testNS},
		{"ps"},
		{"status"},
		{"status", testNS},
		{"version"},
		{"health"},
		{"arrow", "add", testNS},
		{"arrow", "remove", testNS, "--yes"},
		{"arrow", "list"},
		{"arrow", "show", testNS},
		{"collection", "follow", "github.com/user/col"},
		{"collection", "unfollow", "github.com/user/col", "--yes"},
		{"collection", "update", "github.com/user/col"},
		{"collection", "list"},
		{"collection", "show", "github.com/user/col"},
		{"install", testNS},
		{"run", testNS},
		{"stop", testNS},
		{"update", testNS},
		{"uninstall", testNS, "--yes"},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "cli.yaml")
			_, err := runCLIConfig(t, cfg, append(args, "--server", "ftp://nope")...)
			assert.Error(t, err)
		})
	}
}

func TestContextCommands_CorruptConfigErrors(t *testing.T) {
	testCases := [][]string{
		{"context", "add", "x", "--ctx-server", "tcp://a:1"},
		{"context", "use", "x"},
		{"context", "list"},
		{"context", "current"},
		{"context", "show", "x"},
		{"context", "remove", "x"},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "cli.yaml")
			require.NoError(t, os.WriteFile(cfg, []byte("contexts: [broken"), 0o600))
			_, err := runCLIConfig(t, cfg, args...)
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
	root.SetArgs([]string{"context", "list"})
	assert.Error(t, root.Execute())
}

func TestUninstall_NonTTYWithoutForceRefuses(t *testing.T) {
	_, err := runCLI(t, &fakeDaemon{t: t}, "uninstall", testNS)
	require.Error(t, err)
	assert.Equal(t, 2, commands.ExitCode(err))
}

func TestStatus_TableShowsLastReturn(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"namespace":"` + testNS +
			`","state":"ready","last_return":{"method":"_install","outcome":"success"}}}`))
	})
	out, err := runWith(t, handler, noTTY(), nil, "status", testNS, "-o", "table")
	require.NoError(t, err)
	assert.Contains(t, out, "_install")
	assert.Contains(t, out, "success")
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

func TestInstall_TTYStreamClosesWithoutTerminal(t *testing.T) {
	f := &fakeDaemon{t: t} // empty script: stream closes, model never done
	_, err := runWith(t, f.handler(), withTTY(), strings.NewReader(""),
		"install", testNS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stream closed before install completed")
}

func TestContextCurrent_MissingActiveErrors(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	raw := "active_context: ghost\ncontexts:\n  - name: x\n    server: tcp://a:1\n"
	require.NoError(t, os.WriteFile(cfg, []byte(raw), 0o600))

	_, err := runCLIConfig(t, cfg, "context", "current")
	assert.Error(t, err)
}

// ─── context edge cases ──────────────────────────────────────────────────────

func TestContext_AddDuplicateErrors(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	_, err := runCLIConfig(t, cfg, "context", "add", "r", "--ctx-server", "tcp://a:1")
	require.NoError(t, err)
	_, err = runCLIConfig(t, cfg, "context", "add", "r", "--ctx-server", "tcp://a:2")
	assert.Error(t, err)
}

func TestContext_AddWithUseActivates(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	_, err := runCLIConfig(t, cfg, "context", "add", "r", "--ctx-server", "tcp://a:1", "--use")
	require.NoError(t, err)

	out, err := runCLIConfig(t, cfg, "context", "current")
	require.NoError(t, err)
	assert.Contains(t, out, "r")
}

func TestContext_UseUnknownErrors(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	_, err := runCLIConfig(t, cfg, "context", "use", "ghost")
	assert.Error(t, err)
}

func TestContext_ShowUnknownErrors(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	_, err := runCLIConfig(t, cfg, "context", "show", "ghost")
	assert.Error(t, err)
}

func TestContext_RemoveActiveNeedsForce(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	_, err := runCLIConfig(t, cfg, "context", "remove", "local")
	assert.Error(t, err)

	_, err = runCLIConfig(t, cfg, "context", "remove", "local", "--force")
	assert.NoError(t, err)
}
