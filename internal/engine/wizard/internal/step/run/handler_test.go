package run_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/runtime"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	steprun "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/run"
)

const testNSKey = "test/user/repo/arrow"

func newTestHandler(t *testing.T) wizstep.Handler[domainstep.RunStep] {
	t.Helper()
	rt, err := runtime.New()
	require.NoError(t, err)
	return steprun.NewHandler(rt)
}

func testReq() wizstep.Request {
	return wizstep.Request{
		NSKey:   testNSKey,
		WorkDir: os.TempDir(),
		Vars:    map[string]string{},
	}
}

func TestHandler_Execute_Success(t *testing.T) {
	h := newTestHandler(t)
	s := domainstep.NewRunStep("echo", "echo hello", false, "5s", true)

	err := h.Execute(context.Background(), testReq(), s)

	require.NoError(t, err)
}

func TestHandler_Execute_NonZeroExit(t *testing.T) {
	h := newTestHandler(t)
	s := domainstep.NewRunStep("fail", "false", false, "5s", true)

	err := h.Execute(context.Background(), testReq(), s)

	require.Error(t, err)
	assert.ErrorIs(t, err, steprun.ErrNonZeroExit)
}

func TestHandler_Execute_Timeout(t *testing.T) {
	h := newTestHandler(t)
	s := domainstep.NewRunStep("sleep", "sleep 10", false, "50ms", true)

	err := h.Execute(context.Background(), testReq(), s)

	require.Error(t, err)
}

func TestHandler_Execute_EmitsPIDEvent(t *testing.T) {
	h := newTestHandler(t)
	var emitted []models.Event
	req := wizstep.Request{
		NSKey:   testNSKey,
		WorkDir: os.TempDir(),
		Vars:    map[string]string{},
		Emit:    func(ev models.Event) { emitted = append(emitted, ev) },
	}
	s := domainstep.NewRunStep("echo", "echo hi", false, "5s", true)

	err := h.Execute(context.Background(), req, s)
	require.NoError(t, err)

	require.Len(t, emitted, 1)
	assert.Equal(t, models.EventKindPID, emitted[0].Kind)
	assert.Greater(t, emitted[0].PID, 0)
}

func TestHandler_Execute_ContextCancelled(t *testing.T) {
	h := newTestHandler(t)
	started := make(chan struct{}, 1)
	req := wizstep.Request{
		NSKey:   testNSKey,
		WorkDir: os.TempDir(),
		Vars:    map[string]string{},
		Emit: func(ev models.Event) {
			if ev.Kind == models.EventKindPID {
				started <- struct{}{}
			}
		},
	}
	s := domainstep.NewRunStep("sleep", "sleep 10", false, "30s", true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- h.Execute(ctx, req, s) }()

	timeout, timeoutCancel := context.WithTimeout(context.Background(), 2e9)
	defer timeoutCancel()
	select {
	case <-started:
	case <-timeout.Done():
		t.Fatal("process did not start within timeout")
	}

	cancel()
	require.Error(t, <-errCh)
}

func TestHandler_Execute_InvalidTimeout_ReturnsError(t *testing.T) {
	h := newTestHandler(t)
	s := domainstep.NewRunStep("echo", "echo hi", false, "bad-timeout", true)

	err := h.Execute(context.Background(), testReq(), s)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timeout")
}

func TestHandler_Execute_NilEmit_NoPanic(t *testing.T) {
	h := newTestHandler(t)
	s := domainstep.NewRunStep("echo", "echo hello", false, "5s", true)

	err := h.Execute(context.Background(), testReq(), s)

	require.NoError(t, err)
}

func uiRequest(
	vars map[string]string,
	workDir string,
	events *[]models.Event,
) wizstep.Request {
	return wizstep.Request{
		NSKey:   testNSKey,
		WorkDir: workDir,
		Vars:    vars,
		Emit:    func(ev models.Event) { *events = append(*events, ev) },
	}
}

func kinds(
	events []models.Event,
) []models.EventKind {
	got := make([]models.EventKind, len(events))
	for i, ev := range events {
		got[i] = ev.Kind
	}
	return got
}

func listenVars() map[string]string {
	return map[string]string{domain.VarArrowUIListen: "/run/x.sock"}
}

func TestHandler_Execute_UIListenOpensBeforeTheProcessAndClosesAfter(t *testing.T) {
	var events []models.Event
	s := domainstep.NewRunStep("chat", "echo hi", false, "5s", true).WithUI(domainstep.UIOptions{Title: "Chat"})

	err := newTestHandler(t).Execute(t.Context(), uiRequest(listenVars(), os.TempDir(), &events), s)

	require.NoError(t, err)
	require.Equal(t, []models.EventKind{models.EventKindSurface, models.EventKindPID, models.EventKindSurfaceClosed}, kinds(events))
	assert.Equal(t, &domainRuntime.Surface{Title: "Chat", Mode: domainRuntime.SurfaceModeListen, Path: "/"}, events[0].Surface)
}

func TestHandler_Execute_UIClosesWhenTheProcessFails(t *testing.T) {
	var events []models.Event
	s := domainstep.NewRunStep("chat", "false", false, "5s", true).WithUI(domainstep.UIOptions{})

	err := newTestHandler(t).Execute(t.Context(), uiRequest(listenVars(), os.TempDir(), &events), s)

	require.ErrorIs(t, err, steprun.ErrNonZeroExit)
	assert.Equal(t, models.EventKindSurfaceClosed, events[len(events)-1].Kind)
}

func TestHandler_Execute_UIClosesWhenTheProcessTimesOut(t *testing.T) {
	var events []models.Event
	s := domainstep.NewRunStep("chat", "sleep 10", false, "50ms", true).WithUI(domainstep.UIOptions{})

	err := newTestHandler(t).Execute(t.Context(), uiRequest(listenVars(), os.TempDir(), &events), s)

	require.Error(t, err)
	assert.Equal(t, models.EventKindSurfaceClosed, events[len(events)-1].Kind)
}

func TestHandler_Execute_UIClosesWhenTheRunIsCancelled(t *testing.T) {
	closed := make(chan struct{})
	started := make(chan struct{})
	req := wizstep.Request{
		NSKey:   testNSKey,
		WorkDir: os.TempDir(),
		Vars:    listenVars(),
		Emit: func(ev models.Event) {
			if ev.Kind == models.EventKindPID {
				close(started)
			}
			if ev.Kind == models.EventKindSurfaceClosed {
				close(closed)
			}
		},
	}
	s := domainstep.NewRunStep("chat", "sleep 10", false, "30s", true).WithUI(domainstep.UIOptions{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- newTestHandler(t).Execute(ctx, req, s) }()
	<-started
	cancel()

	require.Error(t, <-errCh)
	select {
	case <-closed:
	default:
		t.Fatal("surface was not reported closed after cancel")
	}
}

func TestHandler_Execute_UIClosesWhenTheProcessCannotStart(t *testing.T) {
	var events []models.Event
	s := domainstep.NewRunStep("chat", "echo hi", false, "5s", true).WithUI(domainstep.UIOptions{})
	h := steprun.NewHandler(&capturingRuntime{})

	err := h.Execute(t.Context(), uiRequest(listenVars(), os.TempDir(), &events), s)

	require.Error(t, err)
	require.Equal(t, []models.EventKind{models.EventKindSurface, models.EventKindSurfaceClosed}, kinds(events))
}

func TestHandler_Execute_UIListenWithoutProvisionedSocket(t *testing.T) {
	var events []models.Event
	s := domainstep.NewRunStep("chat", "echo hi", false, "5s", true).WithUI(domainstep.UIOptions{})

	err := newTestHandler(t).Execute(t.Context(), uiRequest(map[string]string{}, os.TempDir(), &events), s)

	require.ErrorIs(t, err, steprun.ErrNoSocket)
	assert.Empty(t, events, "nothing opens and nothing starts")
}

func TestHandler_Execute_WithoutUIEmitsNoSurfaceEvents(t *testing.T) {
	var events []models.Event
	s := domainstep.NewRunStep("echo", "echo hi", false, "5s", true)

	require.NoError(t, newTestHandler(t).Execute(t.Context(), uiRequest(map[string]string{}, os.TempDir(), &events), s))

	assert.Equal(t, []models.EventKind{models.EventKindPID}, kinds(events))
}

func TestHandler_Execute_UIStaticIsReadyAndAbsolute(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "dist"), 0o755))
	wantDir, err := filepath.EvalSymlinks(filepath.Join(work, "dist")) // t.TempDir is symlinked on macOS
	require.NoError(t, err)
	var events []models.Event
	s := domainstep.NewRunStep("keepalive", "echo hi", false, "5s", true).
		WithUI(domainstep.UIOptions{Static: "./dist", Path: "/start"})

	require.NoError(t, newTestHandler(t).Execute(t.Context(), uiRequest(nil, work, &events), s))

	assert.Equal(t, &domainRuntime.Surface{
		Mode:  domainRuntime.SurfaceModeStatic,
		Path:  "/start",
		Dir:   wantDir,
		Ready: true,
	}, events[0].Surface)
	assert.Equal(t, models.EventKindSurfaceClosed, events[len(events)-1].Kind)
}

func TestHandler_Execute_UIStaticRejectsEscapeAndMissing(t *testing.T) {
	work := t.TempDir()
	for _, dir := range []string{"../outside", "/etc", "./nope"} {
		var events []models.Event
		s := domainstep.NewRunStep("keepalive", "echo hi", false, "5s", true).WithUI(domainstep.UIOptions{Static: dir})

		err := newTestHandler(t).Execute(t.Context(), uiRequest(nil, work, &events), s)

		require.ErrorIs(t, err, steprun.ErrBadStaticDir, dir)
		assert.Empty(t, events, dir)
	}
}

func TestHandler_Execute_UIStaticRejectsSymlinkEscape(t *testing.T) {
	work, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(work, "link")))
	var events []models.Event
	s := domainstep.NewRunStep("keepalive", "echo hi", false, "5s", true).WithUI(domainstep.UIOptions{Static: "./link"})

	err := newTestHandler(t).Execute(t.Context(), uiRequest(nil, work, &events), s)

	require.ErrorIs(t, err, steprun.ErrBadStaticDir)
}

func TestHandler_Execute_UIStaticExpandsVariables(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "site"), 0o755))
	want, err := filepath.EvalSymlinks(filepath.Join(work, "site"))
	require.NoError(t, err)
	var events []models.Event
	s := domainstep.NewRunStep("keepalive", "echo hi", false, "5s", true).WithUI(domainstep.UIOptions{Static: "./${SITE}"})

	require.NoError(t, newTestHandler(t).Execute(t.Context(), uiRequest(map[string]string{"SITE": "site"}, work, &events), s))

	assert.Equal(t, want, events[0].Surface.Dir)
}

func TestHandler_Execute_UINilEmit_NoPanic(t *testing.T) {
	s := domainstep.NewRunStep("chat", "echo hi", false, "5s", true).WithUI(domainstep.UIOptions{})
	req := wizstep.Request{NSKey: testNSKey, WorkDir: os.TempDir(), Vars: listenVars()}

	require.NotPanics(t, func() {
		require.NoError(t, newTestHandler(t).Execute(t.Context(), req, s))
	})
}

// capturingRuntime records the config a step is started with, so the two
// properties that matter can be asserted without running a shell: what reaches
// the process, and what does not. sh and cmd.exe disagree on every construct a
// shell-based assertion would use, and neither disagreement is Quiver's.
type capturingRuntime struct{ got *runtime.Config }

func (r *capturingRuntime) Start(
	_ context.Context,
	cfg *runtime.Config,
) (runtime.Process, error) {
	r.got = cfg
	return nil, errors.New("captured")
}

func (r *capturingRuntime) SignalPID(
	_ context.Context,
	_ int,
	_ domainstep.SignalKind,
) error {
	return nil
}

func (r *capturingRuntime) ProcessAlive(_ int) bool { return false }

func capturedConfig(
	t *testing.T,
	vars map[string]string,
	command string,
) *runtime.Config {
	t.Helper()

	rt := &capturingRuntime{}
	h := steprun.NewHandler(rt)
	req := wizstep.Request{NSKey: testNSKey, WorkDir: t.TempDir(), Vars: vars}
	s := domainstep.NewRunStep("captured", command, false, "5s", true)

	_ = h.Execute(context.Background(), req, s)

	require.NotNil(t, rt.got, "the handler must have started a process")
	return rt.got
}

// The shell must never receive a Quiver token — substitution happens before the
// command leaves the handler, on every platform.
func TestHandler_Execute_CommandReachesTheShellAlreadyExpanded(t *testing.T) {
	cfg := capturedConfig(
		t,
		map[string]string{"ns/path.KEY": "resolved"},
		"run ${ns/path.KEY} && keep $HOME ${UNKNOWN}",
	)

	require.Len(t, cfg.Command, 1)
	assert.Equal(t, "run resolved && keep $HOME ${UNKNOWN}", cfg.Command[0])
}

// Quiver's variables are a manifest concern; the process environment stays the
// OS's. This pins the removal of config.Env = req.Vars structurally, rather
// than by asking a shell what it can see.
func TestHandler_Execute_VarsNeverReachTheProcessEnvironment(t *testing.T) {
	cfg := capturedConfig(t, map[string]string{"QUIVER_VAR": "quiver-value"}, "echo hi")

	assert.Empty(t, cfg.Env)
}

// errRuntime is a runtime.Runtime whose Start always fails.
type errRuntime struct{ err error }

func (r errRuntime) Start(
	_ context.Context,
	_ *runtime.Config,
) (runtime.Process, error) {
	return nil, r.err
}

func (r errRuntime) SignalPID(
	_ context.Context,
	_ int,
	_ domainstep.SignalKind,
) error {
	return nil
}

func (r errRuntime) ProcessAlive(_ int) bool { return false }

func TestHandler_Execute_StartError(t *testing.T) {
	wantErr := errors.New("start failed")
	h := steprun.NewHandler(errRuntime{err: wantErr})
	s := domainstep.NewRunStep("echo", "echo hi", false, "5s", true)

	err := h.Execute(context.Background(), testReq(), s)

	require.ErrorIs(t, err, wantErr)
}

// runCapture executes command with vars and returns what the shell wrote to
// out.txt, so assertions are on the bytes the command actually received.
//
// POSIX only. The runtime wraps a step in `sh -c` on unix and `cmd.exe /C` on
// Windows, and these callers assert on printf, single-quote quoting and $VAR —
// none of which mean the same thing to cmd.exe. That is a difference between
// shells, not a difference in Quiver, so asserting it on Windows would test the
// wrong thing. What Quiver actually guarantees — an already-expanded command and
// an untouched environment — is asserted on every platform through
// capturedConfig, which needs no shell at all.
func runCapture(
	t *testing.T,
	vars map[string]string,
	command string,
) string {
	t.Helper()

	if goruntime.GOOS == "windows" {
		t.Skip("asserts POSIX shell syntax; the guarantees themselves are covered by capturedConfig")
	}

	workDir := t.TempDir()
	h := newTestHandler(t)
	req := wizstep.Request{NSKey: testNSKey, WorkDir: workDir, Vars: vars}
	s := domainstep.NewRunStep("capture", command+" > out.txt", false, "10s", true)

	require.NoError(t, h.Execute(context.Background(), req, s))

	data, err := os.ReadFile(filepath.Join(workDir, "out.txt"))
	require.NoError(t, err)
	return string(data)
}

func TestHandler_Execute_ExpandsQuiverVariables(t *testing.T) {
	out := runCapture(
		t,
		map[string]string{"MY_VAR": "quiver-value"},
		`printf '%s' '${MY_VAR}'`,
	)

	assert.Equal(t, "quiver-value", out)
}

func TestHandler_Execute_ExpandsNamespacedExport(t *testing.T) {
	const key = "quiver.test/quiver-test/tool-exporter.EXPORTED_BIN"

	out := runCapture(
		t,
		map[string]string{key: "/vault/tool-exporter/quiver-exporter-bin"},
		`printf '%s' '${`+key+`}'`,
	)

	assert.Equal(t, "/vault/tool-exporter/quiver-exporter-bin", out)
}

func TestHandler_Execute_UnknownVariableReachesShellVerbatim(t *testing.T) {
	out := runCapture(t, map[string]string{}, `printf '%s' '${NOT_A_QUIVER_VAR}'`)

	assert.Equal(t, "${NOT_A_QUIVER_VAR}", out)
}

func TestHandler_Execute_ShellConstructsSurviveVerbatim(t *testing.T) {
	out := runCapture(t, map[string]string{"A": "alpha"}, `printf '%s' '$HOME $(uname) $? $$ "$@" \$'`)

	assert.Equal(t, `$HOME $(uname) $? $$ "$@" \$`, out)
}

func TestHandler_Execute_ProcessEnvironmentStaysTheOSs(t *testing.T) {
	t.Setenv("QUIVER_EXPAND_PROBE", "os-value")

	out := runCapture(t, map[string]string{"A": "alpha"}, `printf '%s' "$QUIVER_EXPAND_PROBE"`)

	assert.Equal(t, "os-value", out)
}

// TestHandler_Execute_VarsAreNotInjectedIntoEnvironment pins the removal of
// config.Env = req.Vars: Quiver's variables are a manifest concern, so a step
// that asks the shell for one must get nothing back.
func TestHandler_Execute_VarsAreNotInjectedIntoEnvironment(t *testing.T) {
	out := runCapture(
		t,
		map[string]string{"QUIVER_ENV_PROBE": "quiver-value"},
		`printf '%s' "$QUIVER_ENV_PROBE"`,
	)

	assert.Empty(t, out)
}

// A run with no workdir would execute in the daemon's own working directory,
// where an uninstall's "rm -rf data" would act on the daemon: it never starts.
func TestHandler_Execute_NoWorkDir_IsRefused(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	req := testReq()
	req.WorkDir = ""
	s := domainstep.NewRunStep("touch", "touch "+marker, false, "5s", true)

	err := newTestHandler(t).Execute(context.Background(), req, s)

	require.ErrorIs(t, err, steprun.ErrNoWorkDir)
	assert.NoFileExists(t, marker)
}
