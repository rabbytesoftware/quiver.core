package commands_test

import (
	"bytes"
	"context"
	"encoding/json"
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

// TestNamespacePanel_UnknownFormatIsUsageError exercises the one branch of
// tui.CodeFor's non-default mapping that a real (not synthetic) error from
// this package still produces: a.runner's tui.ParseFormat failure surfaces
// immediately, ahead of namespacePanel's error-swallowing fetch closure.
func TestNamespacePanel_UnknownFormatIsUsageError(t *testing.T) {
	_, err := runCLI(t, &fakeDaemon{t: t}, testNS, "-o", "xml")
	require.Error(t, err)
	assert.Equal(t, 2, commands.ExitCode(err))
}

// ─── active state ────────────────────────────────────────────────────────────

func TestIsActiveState_DelegatesToRuntimePackage(t *testing.T) {
	assert.True(t, commands.IsActiveState("running"))
	assert.True(t, commands.IsActiveState("installing"))
	assert.False(t, commands.IsActiveState("absent"))
	assert.False(t, commands.IsActiveState("ready"))
}

// ─── namespace dispatch ──────────────────────────────────────────────────────

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

// ─── session + config errors ─────────────────────────────────────────────────
//
// Every case below needs some command wired through commands.Attach that
// still reaches a.session — with every resource command extracted (Task
// 17 finished the cascade Tasks 14-16 started: health -> ps -> list), the
// only one left is the bare `quiver <namespace> <method>` dispatch path,
// so that is the vehicle now.

func TestSession_CorruptConfigErrors(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "cli.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("contexts: [not-a-context"), 0o600))

	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.Attach(root, noTTY())
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{testNS, "backup", "--config", cfgPath})
	assert.Error(t, root.Execute())
}

func TestSession_UnknownContextErrors(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	_, err := runCLIConfig(t, cfg, testNS, "backup", "--context", "ghost")
	assert.Error(t, err)
}

func TestSession_BadServerSchemeErrors(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cli.yaml")
	_, err := runCLIConfig(t, cfg, testNS, "backup", "--server", "ftp://nope")
	assert.Error(t, err)
}

func TestSession_DefaultConfigPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.Attach(root, noTTY())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	// The bare `quiver <ns> <method>` dispatch path reads the default
	// config through a.methodSession without needing an explicit --config
	// flag. The local socket resolves but nothing is listening, so the
	// failure comes back as a connection error rather than a
	// config-loading one, which is what proves the default path was found.
	root.SetArgs([]string{testNS, "backup"})

	err := root.Execute()
	require.Error(t, err)
	assert.Equal(t, client.ExitConnection, commands.ExitCode(err))
}

func TestSession_EnsureDaemonCalledForUnixServers(t *testing.T) {
	called := false
	deps := noTTY()
	deps.EnsureDaemon = func(context.Context) error { called = true; return errors.New("boot failed") }

	_, err := runWith(t, nil, deps, nil,
		testNS, "backup", "--server", "unix:///nonexistent/quiver.sock")
	require.Error(t, err)
	assert.True(t, called)
	assert.Contains(t, err.Error(), "boot failed")
}

func TestLoadConfig_NoHomeErrors(t *testing.T) {
	testutil.RequireUnix(t)

	t.Setenv("HOME", "")

	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.Attach(root, noTTY())
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{testNS, "backup"})
	assert.Error(t, root.Execute())
}

// ─── a.session / a.loadConfig / a.runner coverage ────────────────────────────
//
// The cases above exercise the *session* package's Client (via
// a.methodSession, the two-arg dispatch's implementation) — the CLI's real
// production behavior. But commands.go still carries its own, now-vestigial
// a.session/a.loadConfig/a.runner, reachable only through namespacePanel
// (the one-arg `quiver <namespace>` path), which swallows every failure
// from them rather than propagating it (see namespacePanel's doc comment).
// These cases exist to keep that soon-to-retire code exercised until Task
// 20 deletes it in favor of commandTree's t.sess/t.rb, so they assert the
// graceful fallback (no error, empty Methods) rather than a propagated one.

func TestNamespacePanel_OmitsMethodsWhenConfigIsCorrupt(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "cli.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("contexts: [not-a-context"), 0o600))

	out, err := runCLIConfig(t, cfgPath, testNS, "-o", "json")
	require.NoError(t, err)

	var panel commands.Panel
	require.NoError(t, json.Unmarshal([]byte(out), &panel))
	assert.Empty(t, panel.Methods)
}

func TestNamespacePanel_OmitsMethodsWhenServerSchemeInvalid(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "cli.yaml")

	out, err := runCLIConfig(t, cfgPath, testNS, "--server", "ftp://nope", "-o", "json")
	require.NoError(t, err)

	var panel commands.Panel
	require.NoError(t, json.Unmarshal([]byte(out), &panel))
	assert.Empty(t, panel.Methods)
}

func TestNamespacePanel_EnsureDaemonCalledForUnixServer(t *testing.T) {
	called := false
	deps := noTTY()
	deps.EnsureDaemon = func(context.Context) error { called = true; return errors.New("boot failed") }

	out, err := runWith(t, nil, deps, nil,
		testNS, "-o", "json", "--server", "unix:///nonexistent/quiver.sock")
	require.NoError(t, err)
	assert.True(t, called)

	var panel commands.Panel
	require.NoError(t, json.Unmarshal([]byte(out), &panel))
	assert.Empty(t, panel.Methods)
}

func TestNamespacePanel_DefaultConfigPathOmitsMethods(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.Attach(root, noTTY())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	// No --config flag: a.loadConfig falls back to config.DefaultPath()
	// under the temp HOME. The default local socket resolves but nothing
	// is listening, so the manifest fetch fails and namespacePanel falls
	// back to the lifecycle-only view rather than erroring.
	root.SetArgs([]string{testNS, "-o", "json"})

	require.NoError(t, root.Execute())

	var panel commands.Panel
	require.NoError(t, json.Unmarshal(out.Bytes(), &panel))
	assert.Empty(t, panel.Methods)
	assert.NotEmpty(t, panel.Lifecycle)
}

func TestNamespacePanel_NoHomeOmitsMethods(t *testing.T) {
	testutil.RequireUnix(t)

	t.Setenv("HOME", "")

	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.Attach(root, noTTY())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{testNS, "-o", "json"})

	require.NoError(t, root.Execute())

	var panel commands.Panel
	require.NoError(t, json.Unmarshal(out.Bytes(), &panel))
	assert.Empty(t, panel.Methods)
}
