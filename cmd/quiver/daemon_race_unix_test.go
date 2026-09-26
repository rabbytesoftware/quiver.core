//go:build !windows

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// daemonRaceModeEnv switches this test binary into a real `quiver daemon`
// process, sharing $QUIVER_HOME and --host with a sibling started at the same
// instant. Only a real subprocess reproduces the bug this guards: the
// exclusivity check and the sqlite migration it must precede both live deep
// inside process startup, not behind any seam an in-process call can race.
const daemonRaceModeEnv = "QUIVER_TEST_DAEMON_RACE_MODE"

// daemonRaceHostEnv carries the shared --host both siblings bind, since
// TestMain's child path has no access to the parent's constructed value.
const daemonRaceHostEnv = "QUIVER_TEST_DAEMON_RACE_HOST"

// runDaemonRaceChild makes this test binary behave exactly like `quiver
// daemon --host <QUIVER_TEST_DAEMON_RACE_HOST>` under the real cobra command
// tree, then reports the outcome the same way cmd/quiver's own main() does.
func runDaemonRaceChild() {
	root := newRootCmd()
	root.SetArgs([]string{"daemon", "--host", os.Getenv(daemonRaceHostEnv)})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "quiver:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// raceChild is one sibling's process handle and captured output.
type raceChild struct {
	cmd  *exec.Cmd
	out  *syncBuffer
	done chan error
}

// syncBuffer lets the test poll a still-running child's combined output
// without racing its own writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func startRaceChild(t *testing.T, home, host string) *raceChild {
	t.Helper()

	self, err := os.Executable()
	require.NoError(t, err)

	cmd := exec.Command(self) // #nosec G204 -- this test binary's own path
	cmd.Env = append(os.Environ(),
		daemonRaceModeEnv+"=1",
		daemonRaceHostEnv+"="+host,
		"QUIVER_HOME="+home,
	)

	out := &syncBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out

	require.NoError(t, cmd.Start())

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	return &raceChild{cmd: cmd, out: out, done: done}
}

// TestDaemonRace_FirstBoot_LoserExitsCleanlyWithoutTouchingTheEventStore is
// the regression test for the first-boot double-launch bug: two daemons
// started at the same instant against the same fresh, shared QUIVER_HOME and
// the same --host socket path, exactly as a first Quiver Desktop launch (no
// tauri-plugin-single-instance, nothing else preventing two OS processes) can
// spawn two sidecars for. Before PrepareGateway existed, internal.New's
// unconditional adapter construction meant both processes reached
// db.AutoMigrate regardless of which one could ever have bound the socket;
// the loser crashed racing the winner to CREATE TABLE the same schema
// ("SQL logic error: table events already exists"), exit code 1, rather than
// retiring cleanly. This asserts the loser now exits via the gateway's own
// exclusivity check instead, before ever constructing a Container.
func TestDaemonRace_FirstBoot_LoserExitsCleanlyWithoutTouchingTheEventStore(t *testing.T) {
	home := t.TempDir()

	// A short path outside t.TempDir(): the test name alone can push that
	// path past macOS's UNIX_PATH_MAX (104 bytes), which fails the bind with
	// an unrelated "invalid argument" before the exclusivity check this test
	// is about ever runs.
	sockFile, err := os.CreateTemp("", "qv-race-*.sock")
	require.NoError(t, err)
	sockPath := sockFile.Name()
	require.NoError(t, sockFile.Close())
	require.NoError(t, os.Remove(sockPath))
	t.Cleanup(func() { _ = os.Remove(sockPath) })
	host := "unix://" + sockPath

	a := startRaceChild(t, home, host)
	b := startRaceChild(t, home, host)

	const (
		pollInterval = 20 * time.Millisecond
		bootTimeout  = 10 * time.Second
	)

	deadline := time.Now().Add(bootTimeout)
	var aExited, bExited bool
	var aErr, bErr error

	for time.Now().Before(deadline) && !aExited && !bExited {
		select {
		case aErr = <-a.done:
			aExited = true
		default:
		}
		select {
		case bErr = <-b.done:
			bExited = true
		default:
		}
		if !aExited && !bExited {
			time.Sleep(pollInterval)
		}
	}

	require.True(t, aExited || bExited, "at least one sibling must have exited within %s (outputs: a=%q b=%q)", bootTimeout, a.out.String(), b.out.String())
	require.False(t, aExited && bExited, "exactly one sibling must lose the race and exit while the other keeps running as the daemon (outputs: a=%q b=%q)", a.out.String(), b.out.String())

	winner, loser := b, a
	loserErr := aErr
	if bExited {
		winner, loser = a, b
		loserErr = bErr
	}

	require.Error(t, loserErr, "the losing sibling must exit non-zero")
	loserOut := loser.out.String()
	assert.Contains(t, strings.ToLower(loserOut), "gateway",
		"the loser must fail via the gateway's own exclusivity check")
	assert.NotContains(t, strings.ToLower(loserOut), "sql logic error",
		"the loser must never reach the sqlite migration at all, not just survive losing it")
	assert.NotContains(t, strings.ToLower(loserOut), "already exists",
		"the loser must never reach CREATE TABLE, the exact old crash signature")
	assert.NotContains(t, strings.ToLower(loserOut), "panic",
		"the loser must exit through its own error return, not a crash")

	require.NoError(t, winner.cmd.Process.Signal(syscall.SIGTERM))
	winnerErr := <-winner.done
	assert.NoError(t, winnerErr, "the winner must shut down cleanly once signalled (output: %q)", winner.out.String())
}
