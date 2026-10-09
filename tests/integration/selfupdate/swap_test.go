//go:build integration && !windows

package selfupdate_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/core/selfmanifest"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

const (
	oldVersion = "e2e-old"
	oldBuildID = "1001"
	newVersion = "e2e-new"
	newBuildID = "2002"

	coreNS    = "github.com/rabbytesoftware/quiver.core@" + oldVersion
	fixtureNS = "github.com/quiver-test/self-update-fixture@v1"

	githubReleases = "https://github.com/rabbytesoftware/quiver.core/releases/download"
)

// binaries are two real quiver builds that differ only in their stamped
// version and build id.
type binaries struct{ old, new string }

func buildBinaries(t *testing.T) binaries {
	t.Helper()
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	dir := t.TempDir()

	build := func(name, version, buildID string) string {
		out := filepath.Join(dir, name)
		cmd := exec.Command("go", "build",
			"-ldflags", fmt.Sprintf("-X main.version=%s -X main.buildID=%s", version, buildID),
			"-o", out, "./cmd/quiver")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
		return out
	}

	return binaries{old: build("quiver-old", oldVersion, oldBuildID), new: build("quiver-new", newVersion, newBuildID)}
}

// daemon is the old build running as a real process in a home of its own.
type daemon struct {
	t      *testing.T
	home   string
	self   string
	socket string
	done   chan struct{}
}

func startOldDaemon(t *testing.T, bins binaries) *daemon {
	t.Helper()
	// A short path: a unix socket's path is limited to about 100 bytes.
	home, err := os.MkdirTemp("", "qe")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(home) })

	self := filepath.Join(home, "self", "quiver")
	require.NoError(t, os.MkdirAll(filepath.Dir(self), 0o755))
	copyFile(t, bins.old, self)

	logFile, err := os.Create(filepath.Join(home, "daemon.log"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = logFile.Close() })

	cmd := exec.Command(self, "daemon")
	cmd.Env = append(os.Environ(), "QUIVER_HOME="+home, "HOME="+home)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	require.NoError(t, cmd.Start())

	d := &daemon{t: t, home: home, self: self, socket: filepath.Join(home, "quiver.sock"), done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(d.done) }()
	t.Cleanup(d.stop)

	d.waitFor(60*time.Second, "the old daemon to answer", func(c *client.Client) bool { return c.Health(context.Background()) == nil })
	return d
}

func (d *daemon) client() *client.Client {
	c, err := client.New("unix://" + d.socket)
	require.NoError(d.t, err)
	return c
}

func (d *daemon) waitFor(timeout time.Duration, what string, cond func(*client.Client) bool) {
	d.t.Helper()
	require.Eventually(d.t, func() bool { return cond(d.client()) }, timeout, 200*time.Millisecond, what)
}

func (d *daemon) version() (string, string) {
	info, err := d.client().Versions(context.Background())
	if err != nil {
		return "", ""
	}
	return info.Version, info.BuildID
}

func (d *daemon) waitForBuild(timeout time.Duration, version, buildID string) {
	d.t.Helper()
	require.Eventually(d.t, func() bool {
		v, b := d.version()
		return v == version && b == buildID
	}, timeout, 200*time.Millisecond, "a daemon answering as %s (build %s) on %s", version, buildID, d.socket)
}

// stop asks whichever daemon answers on the socket to shut down, through its
// own API, and waits until it is gone.
func (d *daemon) stop() {
	proc, err := d.client().Shutdown(context.Background())
	if err != nil {
		return
	}
	require.Eventually(d.t, func() bool { return syscall.Kill(proc.PID, 0) != nil }, 90*time.Second, 200*time.Millisecond)
}

func (d *daemon) selfLog() string {
	log, _ := os.ReadFile(filepath.Join(d.home, "logs", "self-update.log")) // #nosec G304 -- a temp home this test owns
	return string(log)
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from) // #nosec G304 -- a temp file this test owns
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(to, data, 0o755)) // #nosec G306 -- an executable
}

func fileSum(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- a temp file this test owns
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// releaseServer stands in for the GitHub release the update downloads from:
// the asset for this platform and the release's checksums.txt.
func releaseServer(t *testing.T, asset string) *httptest.Server {
	t.Helper()
	name := fmt.Sprintf("quiver-%s-%s", runtime.GOOS, runtime.GOARCH)
	sums := fmt.Sprintf("%s  ./%s\n", fileSum(t, asset), name)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/"+name):
			http.ServeFile(w, r, asset)
		case strings.HasSuffix(r.URL.Path, "/checksums.txt"):
			_, _ = w.Write([]byte(sums))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// startSupervisedFixture runs a long-lived process under the old daemon and
// returns its pid.
func startSupervisedFixture(t *testing.T, d *daemon) int {
	t.Helper()
	ctx := context.Background()
	c := d.client()
	require.NoError(t, c.SeedArrowManifest(ctx, fixtureNS, kit.ReadFixture(t, "self-update-fixture/arrow.yaml")))
	_, err := c.ExecuteMethod(ctx, fixtureNS, "install", nil)
	require.NoError(t, err)
	d.waitFor(60*time.Second, "the fixture to install", func(c *client.Client) bool {
		rt, err := c.GetRuntime(ctx, fixtureNS)
		return err == nil && rt.State == "ready"
	})
	_, err = c.ExecuteMethod(ctx, fixtureNS, "execute", nil)
	require.NoError(t, err)

	var pid int
	d.waitFor(60*time.Second, "the fixture to run", func(c *client.Client) bool {
		rt, err := c.GetRuntime(ctx, fixtureNS)
		if err != nil || rt.State != "running" || rt.ActiveRun == nil {
			return false
		}
		pid = rt.ActiveRun.PID
		return pid > 0
	})
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	return pid
}

// updateAsMethod is core's own manifest, pointed at the release server, with
// its update steps also offered as a custom method. POST /runtime/{ns}/update
// first re-resolves the target against the git host, which a sandbox without
// github.com cannot answer; a custom method runs the very same steps through
// the same wizard without that lookup.
func updateAsMethod(t *testing.T, srv *httptest.Server) []byte {
	t.Helper()
	manifest := strings.ReplaceAll(string(selfmanifest.Raw()), githubReleases, srv.URL+"/releases/download")
	require.NotEqual(t, string(selfmanifest.Raw()), manifest)

	const updateKey = "      update:\n"
	const fence = "```\n"
	head, steps, found := strings.Cut(manifest, updateKey)
	require.True(t, found)
	steps = strings.TrimSuffix(steps, fence)
	var indented strings.Builder
	for _, line := range strings.SplitAfter(steps, "\n") {
		if line != "" {
			indented.WriteString("  " + line)
		}
	}
	return []byte(head + updateKey + steps +
		"    methods:\n      selfupdate:\n        available_in: [ready]\n        steps:\n" + indented.String() + fence)
}

// triggerUpdate points core's update at the release server and runs it, as a
// client does.
func triggerUpdate(t *testing.T, d *daemon, srv *httptest.Server) {
	t.Helper()
	ctx := context.Background()
	d.waitFor(60*time.Second, "the daemon to register itself", func(c *client.Client) bool {
		_, err := c.GetArrow(ctx, coreNS)
		return err == nil
	})
	require.NoError(t, d.client().SeedArrowManifest(ctx, coreNS, updateAsMethod(t, srv)))

	started, err := d.client().ExecuteMethod(ctx, coreNS, "selfupdate", nil)
	require.NoError(t, err)
	require.True(t, started, "the update run must start")
}

// The whole of the feature, on real processes: the old build's own update
// downloads the new build, the new build replaces the daemon on the same
// socket, and a process the old daemon supervised outlives it.
func TestSelfUpdate_E2E_NewDaemonTakesOverTheSocket(t *testing.T) {
	bins := buildBinaries(t)
	d := startOldDaemon(t, bins)
	version, buildID := d.version()
	require.Equal(t, []string{oldVersion, oldBuildID}, []string{version, buildID})
	fixturePID := startSupervisedFixture(t, d)
	t.Logf("old daemon answers as %s (build %s); supervised fixture pid %d", version, buildID, fixturePID)

	triggerUpdate(t, d, releaseServer(t, bins.new))

	d.waitForBuild(120*time.Second, newVersion, newBuildID)
	newVersionSeen, newBuildSeen := d.version()
	t.Logf("same socket %s now answers as %s (build %s)", d.socket, newVersionSeen, newBuildSeen)
	select {
	case <-d.done:
	case <-time.After(30 * time.Second):
		t.Fatal("the old daemon process is still running")
	}
	assert.Equal(t, fileSum(t, bins.new), fileSum(t, d.self), "the new build is at the self path")
	require.Eventually(t, func() bool {
		left, _ := filepath.Glob(filepath.Join(filepath.Dir(d.self), "quiver.old-*"))
		return len(left) == 0
	}, 30*time.Second, 200*time.Millisecond, "the previous binary is cleaned up")
	require.Eventually(t, func() bool {
		return strings.Contains(d.selfLog(), "new daemon healthy")
	}, 30*time.Second, 200*time.Millisecond, "the updater to log the new daemon healthy")
	t.Logf("self-update.log:\n%s", d.selfLog())

	assert.NoError(t, syscall.Kill(fixturePID, 0), "the supervised process survived the daemon replacing itself")
	d.waitFor(60*time.Second, "the fixture to be found detached", func(c *client.Client) bool {
		rt, err := c.GetRuntime(context.Background(), fixtureNS)
		if err == nil {
			t.Logf("fixture pid %d alive, runtime state %q", fixturePID, rt.State)
		}
		return err == nil && rt.State == "detached"
	})
}

// A new build that never comes up healthy is killed and the previous build is
// put back, serving on the same socket.
func TestSelfUpdate_E2E_UnhealthyNewDaemonRollsBack(t *testing.T) {
	bins := buildBinaries(t)
	d := startOldDaemon(t, bins)
	fixturePID := startSupervisedFixture(t, d)

	// The release asset hands the self-update to the real new build but is
	// itself a program that never serves.
	dud := filepath.Join(t.TempDir(), "dud")
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = self-update ]; then exec %q self-update \"$2\"; fi\nexec sleep 600\n", bins.new)
	require.NoError(t, os.WriteFile(dud, []byte(script), 0o755)) // #nosec G306 -- an executable

	triggerUpdate(t, d, releaseServer(t, dud))

	require.Eventually(t, func() bool { return strings.Contains(d.selfLog(), "rolling back") },
		120*time.Second, 200*time.Millisecond, "the update must give up on the unhealthy build")
	d.waitFor(60*time.Second, "the previous build to answer again", func(c *client.Client) bool {
		info, err := c.Versions(context.Background())
		return err == nil && info.Version == oldVersion && info.BuildID == oldBuildID
	})
	assert.Equal(t, fileSum(t, bins.old), fileSum(t, d.self), "the previous build is back at the self path")
	v, b := d.version()
	t.Logf("socket %s answers as %s (build %s) again; self-update.log:\n%s", d.socket, v, b, d.selfLog())
	assert.NoError(t, syscall.Kill(fixturePID, 0), "the supervised process survived the failed update too")
}
