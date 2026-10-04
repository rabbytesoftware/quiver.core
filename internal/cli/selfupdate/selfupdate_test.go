package selfupdate_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/selfupdate"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

type start struct {
	exe     string
	content string
	args    []string
}

// world is a self directory with an old daemon binary and a new one beside it,
// and fakes for everything that would touch a process.
type world struct {
	t       *testing.T
	dir     string
	self    string
	newBin  string
	stopped bool
	starts  []start
	killed  []int
	healthy func(startCount int) bool
	startFn func(n int) error
	updater *selfupdate.Updater
}

func newWorld(t *testing.T) *world {
	t.Helper()
	dir := t.TempDir()
	w := &world{t: t, dir: dir, self: filepath.Join(dir, "quiver"), newBin: filepath.Join(dir, "download")}
	require.NoError(t, os.WriteFile(w.self, []byte("old"), 0o755)) // #nosec G306 -- a fake executable in a temp dir
	require.NoError(t, os.WriteFile(w.newBin, []byte("new"), 0o644))
	w.healthy = func(int) bool { return true }
	w.updater = &selfupdate.Updater{
		Self: w.self,
		Shutdown: func(context.Context) (dto.ShutdownDTO, error) {
			w.stopped = true
			return dto.ShutdownDTO{PID: 100, Exe: w.self, Args: []string{"daemon", "--host", "unix://"}}, nil
		},
		Alive:     func(pid int) bool { return pid == 100 && !w.stopped },
		Listening: func(context.Context) bool { return false },
		Healthy:   func(context.Context) bool { return w.healthy(len(w.starts)) },
		Start: func(exe string, args []string) (int, error) {
			content, _ := os.ReadFile(exe) // #nosec G304 -- a temp file this test owns
			w.starts = append(w.starts, start{exe: exe, content: string(content), args: args})
			if w.startFn != nil {
				if err := w.startFn(len(w.starts)); err != nil {
					return 0, err
				}
			}
			return 200 + len(w.starts), nil
		},
		Kill:          func(pid int) { w.killed = append(w.killed, pid) },
		StopTimeout:   200 * time.Millisecond,
		HealthTimeout: 200 * time.Millisecond,
		Poll:          time.Millisecond,
	}
	return w
}

func (w *world) content() string {
	w.t.Helper()
	got, err := os.ReadFile(w.self) // #nosec G304 -- a temp file this test owns
	require.NoError(w.t, err)
	return string(got)
}

func (w *world) entries() []string {
	w.t.Helper()
	list, err := os.ReadDir(w.dir)
	require.NoError(w.t, err)
	names := make([]string, 0, len(list))
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

func TestUpdater_Run_SwapsAndStartsTheNewDaemon(t *testing.T) {
	w := newWorld(t)
	require.NoError(t, os.WriteFile(filepath.Join(w.dir, "quiver.old-1"), []byte("stale"), 0o755)) // #nosec G306 -- a fake leftover
	require.NoError(t, os.WriteFile(filepath.Join(w.dir, "quiver.old-9"), []byte("stale"), 0o755)) // #nosec G306 -- a fake leftover

	require.NoError(t, w.updater.Run(context.Background(), w.newBin))

	assert.Equal(t, "new", w.content())
	require.Len(t, w.starts, 1)
	assert.Equal(t, start{exe: w.self, content: "new", args: []string{"daemon", "--host", "unix://"}}, w.starts[0])
	assert.ElementsMatch(t, []string{"quiver", "download"}, w.entries(), "stale asides and the aside of this swap are gone")
	if runtime.GOOS != "windows" {
		info, err := os.Stat(w.self)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode().Perm()&0o100, "the new binary must be executable")
	}
}

func TestUpdater_Run_FirstInstallHasNothingToMoveAside(t *testing.T) {
	w := newWorld(t)
	require.NoError(t, os.Remove(w.self))

	require.NoError(t, w.updater.Run(context.Background(), w.newBin))

	assert.Equal(t, "new", w.content())
}

func TestUpdater_Run_KeepsReadingTheNewBinaryAfterItIsRemoved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("an open file cannot be removed on windows")
	}
	w := newWorld(t)
	shutdown := w.updater.Shutdown
	w.updater.Shutdown = func(ctx context.Context) (dto.ShutdownDTO, error) {
		require.NoError(t, os.Remove(w.newBin))
		return shutdown(ctx)
	}

	require.NoError(t, w.updater.Run(context.Background(), w.newBin))

	assert.Equal(t, "new", w.content())
}

func TestUpdater_Run_UnhealthyNewDaemonRollsBack(t *testing.T) {
	w := newWorld(t)
	w.healthy = func(int) bool { return false }

	err := w.updater.Run(context.Background(), w.newBin)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not healthy")
	assert.Equal(t, "old", w.content())
	assert.Equal(t, []int{201}, w.killed, "only the daemon this updater started is killed")
	require.Len(t, w.starts, 2)
	assert.Equal(t, start{exe: w.self, content: "old", args: []string{"daemon", "--host", "unix://"}}, w.starts[1])
	assert.ElementsMatch(t, []string{"quiver", "download"}, w.entries())
}

func TestUpdater_Run_NewDaemonThatCannotStartRollsBack(t *testing.T) {
	w := newWorld(t)
	boom := errors.New("exec format error")
	w.startFn = func(n int) error {
		if n == 1 {
			return boom
		}
		return nil
	}

	err := w.updater.Run(context.Background(), w.newBin)

	require.ErrorIs(t, err, boom)
	assert.Equal(t, "old", w.content())
	assert.Empty(t, w.killed)
	assert.Len(t, w.starts, 2)
}

func TestUpdater_Run_RollbackThatCannotRestartReportsBoth(t *testing.T) {
	w := newWorld(t)
	w.healthy = func(int) bool { return false }
	boom := errors.New("cannot start old")
	w.startFn = func(n int) error {
		if n == 2 {
			return boom
		}
		return nil
	}

	err := w.updater.Run(context.Background(), w.newBin)

	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "not healthy")
	assert.Equal(t, "old", w.content())
}

func TestUpdater_Run_AsideSweptBeforeRollbackIsReported(t *testing.T) {
	w := newWorld(t)
	w.healthy = func(int) bool { return false }
	w.startFn = func(int) error {
		matches, _ := filepath.Glob(filepath.Join(w.dir, "quiver.old-*"))
		for _, m := range matches {
			require.NoError(t, os.Remove(m))
		}
		return nil
	}

	err := w.updater.Run(context.Background(), w.newBin)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "restore")
}

func TestUpdater_Run_ShutdownRefusedChangesNothing(t *testing.T) {
	w := newWorld(t)
	boom := errors.New("connection refused")
	w.updater.Shutdown = func(context.Context) (dto.ShutdownDTO, error) { return dto.ShutdownDTO{}, boom }

	err := w.updater.Run(context.Background(), w.newBin)

	require.ErrorIs(t, err, boom)
	assert.Equal(t, "old", w.content())
	assert.Empty(t, w.starts)
}

func TestUpdater_Run_DaemonThatNeverLeavesIsNotSwapped(t *testing.T) {
	testCases := []struct {
		name      string
		processUp bool
		socketUp  bool
	}{
		{name: "process stays", processUp: true},
		{name: "socket stays", socketUp: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t)
			w.updater.Alive = func(int) bool { return tc.processUp }
			w.updater.Listening = func(context.Context) bool { return tc.socketUp }

			err := w.updater.Run(context.Background(), w.newBin)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "still running")
			assert.Equal(t, "old", w.content())
			assert.Empty(t, w.starts)
		})
	}
}

func TestUpdater_Run_CancelledWhileWaitingStops(t *testing.T) {
	w := newWorld(t)
	w.updater.StopTimeout = time.Minute
	w.updater.Alive = func(int) bool { return true }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := w.updater.Run(ctx, w.newBin)

	require.Error(t, err)
	assert.Equal(t, "old", w.content())
}

func TestUpdater_Run_MissingNewBinaryIsRefusedBeforeShutdown(t *testing.T) {
	w := newWorld(t)

	err := w.updater.Run(context.Background(), filepath.Join(w.dir, "nope"))

	require.Error(t, err)
	assert.False(t, w.stopped, "the daemon is left alone when there is nothing to put in its place")
}

func TestUpdater_Run_UnwritableSelfDirRestartsTheOldDaemon(t *testing.T) {
	w := newWorld(t)
	w.updater.Self = filepath.Join(w.dir, "missing-dir", "quiver")

	err := w.updater.Run(context.Background(), w.newBin)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "stage")
	require.Len(t, w.starts, 1)
	assert.Equal(t, w.self, w.starts[0].exe, "the daemon that was stopped comes back")
}

func TestUpdater_Run_UnreadableNewBinaryRestartsTheOldDaemon(t *testing.T) {
	w := newWorld(t)
	dir := filepath.Join(w.dir, "a-directory")
	require.NoError(t, os.Mkdir(dir, 0o755))

	err := w.updater.Run(context.Background(), dir)

	require.Error(t, err)
	assert.Equal(t, "old", w.content())
	require.Len(t, w.starts, 1)
	assert.Equal(t, w.self, w.starts[0].exe)
}

func TestUpdater_Run_AsideStaysUntilTheNewDaemonIsHealthy(t *testing.T) {
	w := newWorld(t)
	var asideDuringHealth bool
	w.updater.Healthy = func(context.Context) bool {
		matches, _ := filepath.Glob(filepath.Join(w.dir, "quiver.old-*"))
		asideDuringHealth = len(matches) == 1
		return false
	}

	err := w.updater.Run(context.Background(), w.newBin)

	require.Error(t, err)
	assert.True(t, asideDuringHealth, "the previous binary must still be there while health is checked")
	assert.Equal(t, "old", w.content(), "a daemon that died after boot, before health, is rolled back")
}

func TestUpdater_Run_NoDaemonOnTheSocketNamesTheLimit(t *testing.T) {
	w := newWorld(t)
	w.updater.Socket = "/home/u/.quiver/quiver.sock"
	w.updater.Shutdown = func(context.Context) (dto.ShutdownDTO, error) {
		return dto.ShutdownDTO{}, errors.New("connection refused")
	}

	err := w.updater.Run(context.Background(), w.newBin)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "/home/u/.quiver/quiver.sock")
	assert.Contains(t, err.Error(), "default local socket")
	assert.Contains(t, err.Error(), "--host")
}

func TestUpdater_Run_ReportsEveryDaemonItStarts(t *testing.T) {
	w := newWorld(t)
	var recorded []int
	w.updater.Started = func(pid int) { recorded = append(recorded, pid) }

	require.NoError(t, w.updater.Run(context.Background(), w.newBin))
	assert.Equal(t, []int{201}, recorded)

	w = newWorld(t)
	recorded = nil
	w.updater.Started = func(pid int) { recorded = append(recorded, pid) }
	w.healthy = func(int) bool { return false }

	require.Error(t, w.updater.Run(context.Background(), w.newBin))
	assert.Equal(t, []int{202}, recorded, "the killed daemon is not recorded; the restored one is")
}
