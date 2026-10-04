package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/testutil"

	"github.com/rabbytesoftware/quiver.core/internal/cli/daemon"
)

type selfUpdateCalls struct {
	detached [][]string
	ran      []string
}

func fakeSelfUpdateDeps(calls *selfUpdateCalls) selfUpdateDeps {
	return selfUpdateDeps{
		executable: func() (string, error) { return "/new/quiver", nil },
		detach: func(exe string, args ...string) error {
			calls.detached = append(calls.detached, append([]string{exe}, args...))
			return nil
		},
		run: func(_ context.Context, newBin string) error {
			calls.ran = append(calls.ran, newBin)
			return nil
		},
	}
}

func runSelfUpdateCmd(t *testing.T, deps selfUpdateDeps, args ...string) error {
	t.Helper()
	cmd := newSelfUpdateCmd(deps)
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd.ExecuteContext(context.Background())
}

func newBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "quiver-new")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o755)) // #nosec G306 -- a fake executable in a temp dir
	return path
}

func TestSelfUpdateCmd_IsHidden(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"self-update"})
	require.NoError(t, err)

	assert.Equal(t, "self-update", cmd.Name())
	assert.True(t, cmd.Hidden)
	assert.True(t, cmd.Flags().Lookup("detached").Hidden)
}

func TestSelfUpdateCmd_RelaunchesItselfDetached(t *testing.T) {
	calls := &selfUpdateCalls{}
	bin := newBinary(t)

	require.NoError(t, runSelfUpdateCmd(t, fakeSelfUpdateDeps(calls), bin))

	assert.Equal(t, [][]string{{"/new/quiver", "self-update", "--detached", bin}}, calls.detached)
	assert.Empty(t, calls.ran, "the process the update run waits on does not do the swap")
}

func TestSelfUpdateCmd_DetachedDoesTheSwap(t *testing.T) {
	calls := &selfUpdateCalls{}
	bin := newBinary(t)

	require.NoError(t, runSelfUpdateCmd(t, fakeSelfUpdateDeps(calls), "--detached", bin))

	assert.Equal(t, []string{bin}, calls.ran)
	assert.Empty(t, calls.detached)
}

func TestSelfUpdateCmd_RelativePathIsMadeAbsolute(t *testing.T) {
	calls := &selfUpdateCalls{}
	bin := newBinary(t)
	t.Chdir(filepath.Dir(bin))

	require.NoError(t, runSelfUpdateCmd(t, fakeSelfUpdateDeps(calls), "--detached", "quiver-new"))

	require.Len(t, calls.ran, 1)
	assert.True(t, filepath.IsAbs(calls.ran[0]))
}

func TestSelfUpdateCmd_RefusesBadArguments(t *testing.T) {
	dir := t.TempDir()

	testCases := []struct {
		name string
		args []string
	}{
		{name: "no argument", args: nil},
		{name: "two arguments", args: []string{newBinary(t), newBinary(t)}},
		{name: "missing file", args: []string{filepath.Join(dir, "nope")}},
		{name: "directory", args: []string{dir}},
		{name: "unknown flag", args: []string{"--bogus", newBinary(t)}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			calls := &selfUpdateCalls{}

			err := runSelfUpdateCmd(t, fakeSelfUpdateDeps(calls), tc.args...)

			require.Error(t, err)
			assert.Empty(t, calls.detached)
			assert.Empty(t, calls.ran)
		})
	}
}

func TestSelfUpdateCmd_DirectoryErrorIsNamed(t *testing.T) {
	err := runSelfUpdateCmd(t, fakeSelfUpdateDeps(&selfUpdateCalls{}), t.TempDir())

	require.ErrorIs(t, err, errNotAFile)
}

func TestSelfUpdateCmd_FailuresPropagate(t *testing.T) {
	boom := errors.New("boom")
	bin := newBinary(t)

	testCases := []struct {
		name string
		deps func(selfUpdateDeps) selfUpdateDeps
		args []string
	}{
		{
			name: "executable unknown",
			deps: func(d selfUpdateDeps) selfUpdateDeps {
				d.executable = func() (string, error) { return "", boom }
				return d
			},
			args: []string{bin},
		},
		{
			name: "detach fails",
			deps: func(d selfUpdateDeps) selfUpdateDeps {
				d.detach = func(string, ...string) error { return boom }
				return d
			},
			args: []string{bin},
		},
		{
			name: "swap fails",
			deps: func(d selfUpdateDeps) selfUpdateDeps {
				d.run = func(context.Context, string) error { return boom }
				return d
			},
			args: []string{"--detached", bin},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := runSelfUpdateCmd(t, tc.deps(fakeSelfUpdateDeps(&selfUpdateCalls{})), tc.args...)

			require.ErrorIs(t, err, boom)
		})
	}
}

func TestRealSelfUpdateDeps_AreWired(t *testing.T) {
	deps := realSelfUpdateDeps()

	assert.NotNil(t, deps.executable)
	assert.NotNil(t, deps.detach)
	assert.NotNil(t, deps.run)
}

func TestRunSelfUpdate_NoDaemonIsReportedAndLogged(t *testing.T) {
	// On Windows the default endpoint is one machine-wide named pipe: a real
	// daemon answering it would be shut down by this test.
	testutil.RequireUnix(t)
	home := t.TempDir()
	t.Setenv("QUIVER_HOME", home)
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	err := runSelfUpdate(context.Background(), newBinary(t))

	require.Error(t, err)
	logged, readErr := os.ReadFile(filepath.Join(home, "logs", "self-update.log")) // #nosec G304 -- a temp dir this test owns
	require.NoError(t, readErr)
	assert.Contains(t, string(logged), "self-update failed")
}

func TestFollowPID_OnlyFollowsADaemonTheCLIBooted(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "quiver.pid")
	m := &daemon.Manager{PIDFile: pidFile}

	assert.Nil(t, followPID(context.Background(), m), "no pid file: someone else started the daemon, so nothing may name it")

	require.NoError(t, m.RecordPID(100))
	follow := followPID(context.Background(), m)
	require.NotNil(t, follow)
	follow(200)
	pid, err := m.ReadPID()
	require.NoError(t, err)
	assert.Equal(t, 200, pid)
}
