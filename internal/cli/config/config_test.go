package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
)

func load(t *testing.T, dir string) *config.Config {
	t.Helper()
	cfg, err := config.Load(filepath.Join(dir, "cli.yaml"))
	require.NoError(t, err)
	return cfg
}

// ─── Load ────────────────────────────────────────────────────────────────────

func TestLoad_MissingFileYieldsLocalContext(t *testing.T) {
	cfg := load(t, t.TempDir())

	ctx, err := cfg.Active()
	require.NoError(t, err)
	assert.Equal(t, "local", ctx.Name)
	assert.Contains(t, ctx.Server, "unix://")
	assert.Contains(t, ctx.Server, "quiver.sock")
}

func TestLoad_RoundTripsSavedContexts(t *testing.T) {
	dir := t.TempDir()
	cfg := load(t, dir)
	require.NoError(t, cfg.Add(config.Context{Name: "homelab", Server: "tcp://10.0.0.5:40257"}, false))

	reloaded := load(t, dir)
	ctx, err := reloaded.Get("homelab")
	require.NoError(t, err)
	assert.Equal(t, "tcp://10.0.0.5:40257", ctx.Server)
}

func TestLoad_CorruptFileErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cli.yaml")
	require.NoError(t, os.WriteFile(path, []byte("{{{not yaml"), 0o600))

	_, err := config.Load(path)
	assert.Error(t, err)
}

// ─── Add / Get / Remove ──────────────────────────────────────────────────────

func TestAdd_DuplicateNameErrors(t *testing.T) {
	cfg := load(t, t.TempDir())
	require.NoError(t, cfg.Add(config.Context{Name: "x", Server: "tcp://a:1"}, false))

	err := cfg.Add(config.Context{Name: "x", Server: "tcp://b:2"}, false)
	assert.Error(t, err)
}

func TestAdd_EmptyServerErrors(t *testing.T) {
	cfg := load(t, t.TempDir())
	assert.Error(t, cfg.Add(config.Context{Name: "x"}, false))
}

func TestAdd_WithUseActivates(t *testing.T) {
	cfg := load(t, t.TempDir())
	require.NoError(t, cfg.Add(config.Context{Name: "remote", Server: "tcp://a:1"}, true))

	ctx, err := cfg.Active()
	require.NoError(t, err)
	assert.Equal(t, "remote", ctx.Name)
}

func TestGet_UnknownErrors(t *testing.T) {
	cfg := load(t, t.TempDir())
	_, err := cfg.Get("ghost")
	assert.Error(t, err)
}

func TestRemove_ActiveContextRequiresForce(t *testing.T) {
	cfg := load(t, t.TempDir())
	require.NoError(t, cfg.Add(config.Context{Name: "r", Server: "tcp://a:1"}, true))

	assert.Error(t, cfg.Remove("r", false))
	assert.NoError(t, cfg.Remove("r", true))
}

func TestRemove_ForcedActiveFallsBackToFirst(t *testing.T) {
	dir := t.TempDir()
	cfg := load(t, dir)
	require.NoError(t, cfg.Add(config.Context{Name: "r", Server: "tcp://a:1"}, true))
	require.NoError(t, cfg.Remove("r", true))

	ctx, err := cfg.Active()
	require.NoError(t, err)
	assert.Equal(t, "local", ctx.Name)
}

func TestRemove_UnknownErrors(t *testing.T) {
	cfg := load(t, t.TempDir())
	assert.Error(t, cfg.Remove("ghost", false))
}

// ─── Use / Active ────────────────────────────────────────────────────────────

func TestUse_SwitchesAndPersists(t *testing.T) {
	dir := t.TempDir()
	cfg := load(t, dir)
	require.NoError(t, cfg.Add(config.Context{Name: "r", Server: "tcp://a:1"}, false))
	require.NoError(t, cfg.Use("r"))

	reloaded := load(t, dir)
	ctx, err := reloaded.Active()
	require.NoError(t, err)
	assert.Equal(t, "r", ctx.Name)
}

func TestUse_UnknownErrors(t *testing.T) {
	cfg := load(t, t.TempDir())
	assert.Error(t, cfg.Use("ghost"))
}

func TestList_IncludesDefaultLocal(t *testing.T) {
	cfg := load(t, t.TempDir())
	names := []string{}
	for _, c := range cfg.List() {
		names = append(names, c.Name)
	}
	assert.Contains(t, names, "local")
}

// ─── Resolve (flag/env/context precedence) ───────────────────────────────────

func TestResolve_FlagServerWins(t *testing.T) {
	cfg := load(t, t.TempDir())
	server, err := cfg.Resolve("tcp://flag:1", "")
	require.NoError(t, err)
	assert.Equal(t, "tcp://flag:1", server)
}

func TestResolve_NamedContextBeatsActive(t *testing.T) {
	cfg := load(t, t.TempDir())
	require.NoError(t, cfg.Add(config.Context{Name: "other", Server: "tcp://other:1"}, false))

	server, err := cfg.Resolve("", "other")
	require.NoError(t, err)
	assert.Equal(t, "tcp://other:1", server)
}

func TestResolve_FallsBackToActive(t *testing.T) {
	cfg := load(t, t.TempDir())
	server, err := cfg.Resolve("", "")
	require.NoError(t, err)
	assert.Contains(t, server, "unix://")
}

func TestResolve_UnknownContextErrors(t *testing.T) {
	cfg := load(t, t.TempDir())
	_, err := cfg.Resolve("", "ghost")
	assert.Error(t, err)
}

// ─── coverage: remaining branches ────────────────────────────────────────────

func TestDefaultPath_UnderHome(t *testing.T) {
	p, err := config.DefaultPath()
	require.NoError(t, err)
	assert.Contains(t, p, filepath.Join(".quiver", "cli.yaml"))
}

func TestDefaultLocalServer_UnixScheme(t *testing.T) {
	assert.Contains(t, config.DefaultLocalServer(), "unix://")
}

func TestActiveName_DefaultsToLocal(t *testing.T) {
	cfg := load(t, t.TempDir())
	assert.Equal(t, "local", cfg.ActiveName())
}

func TestLoad_EmptyContextListFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cli.yaml")
	require.NoError(t, os.WriteFile(path, []byte("active_context: ghost\ncontexts: []\n"), 0o600))

	cfg, err := config.Load(path)
	require.NoError(t, err)
	ctx, err := cfg.Active()
	require.NoError(t, err)
	assert.Equal(t, "local", ctx.Name)
}

func TestLoad_UnreadableFileErrors(t *testing.T) {
	dir := t.TempDir()
	// A directory where a file is expected produces a read error.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cli.yaml"), 0o750))

	_, err := config.Load(filepath.Join(dir, "cli.yaml"))
	assert.Error(t, err)
}

func TestAdd_EmptyNameErrors(t *testing.T) {
	cfg := load(t, t.TempDir())
	assert.Error(t, cfg.Add(config.Context{Server: "tcp://a:1"}, false))
}

func TestRemove_LastContextRestoresDefault(t *testing.T) {
	dir := t.TempDir()
	cfg := load(t, dir)
	require.NoError(t, cfg.Remove("local", true))

	ctx, err := cfg.Active()
	require.NoError(t, err)
	assert.Equal(t, "local", ctx.Name)
}

func TestSave_UnwritablePathErrors(t *testing.T) {
	dir := t.TempDir()
	readonly := filepath.Join(dir, "readonly")
	require.NoError(t, os.MkdirAll(readonly, 0o500))
	t.Cleanup(func() { _ = os.Chmod(readonly, 0o700) })

	cfg, err := config.Load(filepath.Join(readonly, "cli.yaml"))
	require.NoError(t, err)
	assert.Error(t, cfg.Add(config.Context{Name: "x", Server: "tcp://a:1"}, false))
}

func TestResolve_ActiveMissingErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cli.yaml")
	require.NoError(t, os.WriteFile(path,
		[]byte("active_context: ghost\ncontexts:\n  - name: real\n    server: tcp://a:1\n"), 0o600))

	cfg, err := config.Load(path)
	require.NoError(t, err)
	_, err = cfg.Resolve("", "")
	assert.Error(t, err)
}

func TestDefaultPath_NoHomeErrors(t *testing.T) {
	t.Setenv("HOME", "")
	_, err := config.DefaultPath()
	assert.Error(t, err)
}

func TestDefaultLocalServer_NoHomeFallsBack(t *testing.T) {
	t.Setenv("HOME", "")
	assert.Equal(t, "unix:///tmp/quiver.sock", config.DefaultLocalServer())
}
