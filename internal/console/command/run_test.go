package command_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/console/command"
)

type fakeDaemon struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []string
	auth     []string
}

func newFakeDaemon(
	t *testing.T,
) *fakeDaemon {
	t.Helper()

	d := &fakeDaemon{}
	d.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.requests = append(d.requests, r.Method+" "+r.URL.Path)
		d.auth = append(d.auth, r.Header.Get("Authorization"))
		d.mu.Unlock()
		d.respond(w, r)
	}))
	t.Cleanup(d.server.Close)
	return d
}

func (d *fakeDaemon) respond(
	w http.ResponseWriter,
	r *http.Request,
) {
	switch r.URL.Path {
	case "/v0/health":
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	case "/versions":
		_, _ = w.Write([]byte(`{"success":true,"data":{"version":"9.9","build_id":"7","api":{"supported":["v0"],"latest":"v0"}}}`))
	default:
		_, _ = w.Write([]byte(`{"success":true,"data":null}`))
	}
}

func (d *fakeDaemon) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.requests)
}

func (d *fakeDaemon) executor() command.Executor {
	return command.New(command.Options{
		ServerURI: d.server.URL,
		Version:   "test-build",
	})
}

func run(
	t *testing.T,
	exec command.Executor,
	bearer string,
	line string,
) (command.Result, string, string) {
	t.Helper()

	tokens, err := command.Tokenize(line)
	require.NoError(t, err)
	inv, err := exec.Prepare(tokens, bearer)
	require.NoError(t, err, line)

	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return inv.Run(ctx, &stdout, &stderr), stdout.String(), stderr.String()
}

func TestExecutor_Run_ExecutesTheRealCommandAgainstTheDaemon(t *testing.T) {
	d := newFakeDaemon(t)

	result, stdout, _ := run(t, d.executor(), "", "health")

	assert.Equal(t, command.Result{}, result)
	assert.Contains(t, stdout, "daemon: ok")
	assert.Equal(t, []string{"GET /v0/health"}, d.requests)
}

func TestExecutor_Run_VersionReportsTheDaemonsOwnVersionString(t *testing.T) {
	d := newFakeDaemon(t)

	_, stdout, _ := run(t, d.executor(), "", "version")

	assert.Contains(t, stdout, "client test-build")
	assert.Contains(t, stdout, "daemon 9.9")
}

func TestExecutor_Run_ForwardsTheCallersBearerTokenAndNothingElse(t *testing.T) {
	d := newFakeDaemon(t)

	run(t, d.executor(), "caller-token", "arrow list")
	run(t, d.executor(), "", "arrow list")

	assert.Equal(t, []string{"Bearer caller-token", ""}, d.auth)
}

func TestExecutor_Run_DefaultsToTableOutputAndHonoursAnExplicitFormat(t *testing.T) {
	d := newFakeDaemon(t)

	_, table, _ := run(t, d.executor(), "", "arrow list")
	_, asJSON, _ := run(t, d.executor(), "", "arrow list -o json")

	assert.NotContains(t, table, `"`+"success"+`"`)
	assert.NotContains(t, table, "\x1b[")
	assert.NotContains(t, asJSON, "\x1b[")
}

func TestExecutor_Run_ADestructiveCommandRefusesWithoutYesAndNeverCallsTheDaemon(t *testing.T) {
	d := newFakeDaemon(t)

	result, _, _ := run(t, d.executor(), "", "arrow remove github.com/a/b")

	assert.Equal(t, 2, result.Code)
	assert.Contains(t, result.Err, "--yes")
	assert.Zero(t, d.count())
}

func TestExecutor_Run_YesDoesNotLeakIntoTheNextCall(t *testing.T) {
	d := newFakeDaemon(t)
	exec := d.executor()

	first, _, _ := run(t, exec, "", "arrow remove github.com/a/b --yes")
	second, _, _ := run(t, exec, "", "arrow remove github.com/a/b")

	assert.Equal(t, command.Result{}, first)
	assert.Equal(t, 2, second.Code)
	assert.Equal(t, 1, d.count())
}

func TestExecutor_Run_AnUninstallAlsoNeedsYes(t *testing.T) {
	d := newFakeDaemon(t)

	result, _, _ := run(t, d.executor(), "", "uninstall github.com/a/b")

	assert.Equal(t, 2, result.Code)
	assert.Zero(t, d.count())
}

func TestExecutor_Run_ReportsUsageErrorsWithTheCLIExitCode(t *testing.T) {
	d := newFakeDaemon(t)

	result, _, _ := run(t, d.executor(), "", "arrow add not-a-namespace")

	assert.Equal(t, 2, result.Code)
	assert.NotEmpty(t, result.Err)
	assert.Zero(t, d.count())
}

func TestExecutor_Run_AnUnreachableDaemonExitsWithTheUnreachableCode(t *testing.T) {
	exec := command.New(command.Options{ServerURI: "http://127.0.0.1:1"})

	result, _, _ := run(t, exec, "", "health")

	assert.Equal(t, 3, result.Code)
}

func TestExecutor_Run_NeverReadsOrWritesTheUsersCLIConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("QUIVER_HOME", filepath.Join(home, "quiver"))
	d := newFakeDaemon(t)

	for _, line := range []string{"health", "version", "arrow list", "ps", "list", "status"} {
		run(t, d.executor(), "tok", line)
	}

	entries, err := os.ReadDir(home)
	require.NoError(t, err)
	assert.Empty(t, entries, "the console must not create files under HOME")
}

func TestExecutor_Run_NeverTouchesTheDaemonsStdin(t *testing.T) {
	d := newFakeDaemon(t)
	original := os.Stdin
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = original
		_ = reader.Close()
		_ = writer.Close()
	})
	_, _ = writer.WriteString("y\n")

	result, _, _ := run(t, d.executor(), "", "arrow remove github.com/a/b")

	assert.Equal(t, 2, result.Code)
	assert.Zero(t, d.count())
}

func TestExecutor_Run_EachCallGetsItsOwnCommandTree(t *testing.T) {
	d := newFakeDaemon(t)
	exec := d.executor()
	tokens := []string{"health"}

	first, err := exec.Prepare(tokens, "")
	require.NoError(t, err)
	second, err := exec.Prepare(tokens, "")
	require.NoError(t, err)

	assert.NotSame(t, first, second)
}

func TestExecutor_Run_ConcurrentCallsAreIndependent(t *testing.T) {
	d := newFakeDaemon(t)
	exec := d.executor()
	var wg sync.WaitGroup
	var failures atomic.Int32

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tokens := []string{"health"}
			inv, err := exec.Prepare(tokens, "")
			if err != nil {
				failures.Add(1)
				return
			}
			var out bytes.Buffer
			if inv.Run(context.Background(), &out, &out).Code != 0 || !strings.Contains(out.String(), "daemon: ok") {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Zero(t, failures.Load())
}
