package context_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/context"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
)

// ─── fixtures ────────────────────────────────────────────────────────────────

// newSession builds a session.Session pointed at cfgPath, for a non-TTY
// invocation. Mirrors the pattern in commands/collection/collection_test.go,
// but context never dials a server, so no daemon URL is needed.
func newSession(t *testing.T, cfgPath string) session.Session {
	t.Helper()

	return session.New(
		session.Deps{IsTTYFunc: func() bool { return false }},
		&session.Flags{Config: cfgPath},
	)
}

// newCfgPath returns a fresh, unwritten config path inside a temp dir.
func newCfgPath(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "cli.yaml")
}

// runContext runs the context command tree against an isolated config file,
// in the given output format (empty means the runner's default: json on a
// non-TTY session).
func runContext(
	t *testing.T, cfgPath, outputFormat string, args ...string,
) (string, error) {
	t.Helper()

	cmds := context.New(newSession(t, cfgPath), runner.New(&runner.Flags{Output: outputFormat}))
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

// ─── add ─────────────────────────────────────────────────────────────────────

func TestAdd_ServerFlagBindsToTheContext(t *testing.T) {
	cfgPath := newCfgPath(t)

	_, err := runContext(t, cfgPath, "", "add", "staging", "--server", "tcp://localhost:40257")
	require.NoError(t, err)

	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)

	got, err := cfg.Get("staging")
	require.NoError(t, err)
	assert.Equal(t, "tcp://localhost:40257", got.Server)
}

func TestAdd_CtxServerAliasStillWorks(t *testing.T) {
	cfgPath := newCfgPath(t)

	_, err := runContext(t, cfgPath, "", "add", "legacy", "--ctx-server", "tcp://localhost:1")
	require.NoError(t, err)

	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)

	got, err := cfg.Get("legacy")
	require.NoError(t, err)
	assert.Equal(t, "tcp://localhost:1", got.Server)
}

func TestAdd_MissingServerIsUsageError(t *testing.T) {
	cfgPath := newCfgPath(t)

	_, err := runContext(t, cfgPath, "", "add", "nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--server is required")
}

func TestAdd_DuplicateNameErrors(t *testing.T) {
	cfgPath := newCfgPath(t)

	_, err := runContext(t, cfgPath, "", "add", "r", "--server", "tcp://a:1")
	require.NoError(t, err)

	_, err = runContext(t, cfgPath, "", "add", "r", "--server", "tcp://a:2")
	assert.Error(t, err)
}

func TestAdd_WithUseActivatesImmediately(t *testing.T) {
	cfgPath := newCfgPath(t)

	_, err := runContext(t, cfgPath, "", "add", "r", "--server", "tcp://a:1", "--use")
	require.NoError(t, err)

	out, err := runContext(t, cfgPath, "", "current")
	require.NoError(t, err)
	assert.Contains(t, out, "r")
}

// ─── use ─────────────────────────────────────────────────────────────────────

func TestUse_SwitchesActiveContext(t *testing.T) {
	cfgPath := newCfgPath(t)

	_, err := runContext(t, cfgPath, "", "add", "homelab", "--server", "tcp://10.0.0.5:40257")
	require.NoError(t, err)

	_, err = runContext(t, cfgPath, "", "use", "homelab")
	require.NoError(t, err)

	out, err := runContext(t, cfgPath, "", "current")
	require.NoError(t, err)
	assert.Contains(t, out, "homelab")
}

func TestUse_UnknownNameErrors(t *testing.T) {
	cfgPath := newCfgPath(t)

	_, err := runContext(t, cfgPath, "", "use", "ghost")
	assert.Error(t, err)
}

// ─── list ────────────────────────────────────────────────────────────────────

func TestList_MarksActiveContext(t *testing.T) {
	cfgPath := newCfgPath(t)

	_, err := runContext(t, cfgPath, "", "add", "homelab", "--server", "tcp://10.0.0.5:40257")
	require.NoError(t, err)

	out, err := runContext(t, cfgPath, "table", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "homelab")
	assert.Contains(t, out, config.LocalContextName)
	assert.Contains(t, out, "*")
}

// ─── current ─────────────────────────────────────────────────────────────────

func TestCurrent_ShowsActiveContext(t *testing.T) {
	cfgPath := newCfgPath(t)

	out, err := runContext(t, cfgPath, "table", "current")
	require.NoError(t, err)
	assert.Contains(t, out, config.LocalContextName)
}

func TestCurrent_MissingActiveErrors(t *testing.T) {
	cfgPath := newCfgPath(t)
	raw := "active_context: ghost\ncontexts:\n  - name: x\n    server: tcp://a:1\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(raw), 0o600))

	_, err := runContext(t, cfgPath, "", "current")
	assert.Error(t, err)
}

// ─── show ────────────────────────────────────────────────────────────────────

func TestShow_RendersContext(t *testing.T) {
	cfgPath := newCfgPath(t)

	_, err := runContext(t, cfgPath, "", "add", "r",
		"--server", "tcp://a:1", "--token", "secret", "--insecure")
	require.NoError(t, err)

	out, err := runContext(t, cfgPath, "table", "show", "r")
	require.NoError(t, err)
	assert.Contains(t, out, "tcp://a:1")
	assert.Contains(t, out, "(set)")
	assert.NotContains(t, out, "secret")
	assert.Contains(t, out, "insecure")
}

func TestShow_UnknownNameErrors(t *testing.T) {
	cfgPath := newCfgPath(t)

	_, err := runContext(t, cfgPath, "", "show", "ghost")
	assert.Error(t, err)
}

// ─── remove ──────────────────────────────────────────────────────────────────

func TestRemove_DeletesContext(t *testing.T) {
	cfgPath := newCfgPath(t)

	_, err := runContext(t, cfgPath, "", "add", "r", "--server", "tcp://a:1")
	require.NoError(t, err)

	_, err = runContext(t, cfgPath, "", "remove", "r")
	require.NoError(t, err)

	out, _ := runContext(t, cfgPath, "table", "list")
	assert.NotContains(t, out, "tcp://a:1")
}

func TestRemove_ActiveNeedsForce(t *testing.T) {
	cfgPath := newCfgPath(t)

	_, err := runContext(t, cfgPath, "", "remove", config.LocalContextName)
	assert.Error(t, err)

	_, err = runContext(t, cfgPath, "", "remove", config.LocalContextName, "--force")
	assert.NoError(t, err)
}

// ─── corrupt config ──────────────────────────────────────────────────────────

func TestCommands_CorruptConfigErrors(t *testing.T) {
	testCases := [][]string{
		{"add", "x", "--server", "tcp://a:1"},
		{"use", "x"},
		{"list"},
		{"current"},
		{"show", "x"},
		{"remove", "x"},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cfgPath := newCfgPath(t)
			require.NoError(t, os.WriteFile(cfgPath, []byte("contexts: [broken"), 0o600))

			_, err := runContext(t, cfgPath, "", args...)
			assert.Error(t, err)
		})
	}
}

// ─── mutation payload shapes ─────────────────────────────────────────────────

func TestMutations_EmitStructuredPayloadWithoutADaemon(t *testing.T) {
	cfgPath := newCfgPath(t)

	out, err := runContext(t, cfgPath, "", "add", "homelab", "--server", "tcp://10.0.0.5:40257")
	require.NoError(t, err)
	assertMutation(t, out, "add", "homelab")

	out, err = runContext(t, cfgPath, "", "use", "homelab")
	require.NoError(t, err)
	assertMutation(t, out, "use", "homelab")

	out, err = runContext(t, cfgPath, "", "remove", "homelab", "--force")
	require.NoError(t, err)
	assertMutation(t, out, "remove", "homelab")
}

func TestMutations_TableRendersOneLine(t *testing.T) {
	cfgPath := newCfgPath(t)

	out, err := runContext(t, cfgPath, "table", "add", "homelab", "--server", "tcp://10.0.0.5:40257")
	require.NoError(t, err)
	assert.Contains(t, out, "added homelab")

	out, err = runContext(t, cfgPath, "table", "use", "homelab")
	require.NoError(t, err)
	assert.Contains(t, out, "switched to homelab")
}
