package runtime_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
)

// ─── install/run/stop/uninstall/update ────────────────────────────────────────

func TestInstall_HappyPathPlainOutput(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: installScript()}

	out, err := runCLI(t, f, "json", "install", testNS)
	require.NoError(t, err)

	// Piped, install emits the run payload. The steps it executed are in it,
	// which is what the old line-per-transition output carried.
	var run output.Run
	require.NoError(t, json.Unmarshal([]byte(out), &run), "run payload: %s", out)

	assert.Equal(t, "install", run.Method)
	assert.Equal(t, testNS, run.Subject)
	assert.Equal(t, "success", run.Outcome)
	assert.Equal(t, "ready", run.State)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/install")
}

func TestInstall_NoOpAlreadyInstalled(t *testing.T) {
	f := &fakeDaemon{t: t, mutationStatus: http.StatusOK}

	out, err := runCLI(t, f, "json", "install", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "already installed, nothing to do")
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/install")
}

func TestInstall_FailureReturnsError(t *testing.T) {
	msg := "fetch: 404"
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "absent", LastReturn: &apidto.ReturnDTO{
			Method: "_install", Outcome: "failed",
			Steps: []apidto.StepProgressDTO{{Index: 0, Status: "failed", Error: &msg, Type: "fetch"}},
		}},
	}}

	out, err := runCLI(t, f, "json", "install", testNS)
	require.Error(t, err)

	// A failed run writes no payload: the error is the result, and a payload
	// on stdout would tell a script the opposite.
	assert.Empty(t, out)

	// The error names the step that stopped and what it said, rather than
	// just reporting the daemon's one-word verdict.
	assert.Contains(t, err.Error(), "install "+testNS)
	assert.Contains(t, err.Error(), "failed")
	assert.Contains(t, err.Error(), "step 1/1")
	assert.Contains(t, err.Error(), msg)
	assert.Contains(t, err.Error(), "state absent")

	// runError also exposes the run in full through Run(), for a caller that
	// wants the whole payload rather than the formatted message.
	withRun, ok := err.(interface{ Run() output.Run })
	require.True(t, ok, "install's error must expose Run()")
	assert.Equal(t, "failed", withRun.Run().Outcome)
}

// A run can fail without any step reporting status "failed" — a cancelled
// or otherwise-aborted outcome with no steps at all, say — in which case the
// error omits the "at step" detail rather than inventing one.
func TestInstall_FailureWithoutFailedStepOmitsStepDetail(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "absent", LastReturn: &apidto.ReturnDTO{
			Method: "_install", Outcome: "cancelled",
		}},
	}}

	_, err := runCLI(t, f, "json", "install", testNS)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "at step")
	assert.Contains(t, err.Error(), "cancelled")
}

// A step reported twice with the same status (e.g. a redundant snapshot
// push) must not re-render as a second transition.
func TestInstall_DuplicateStepEventIsSkipped(t *testing.T) {
	step := apidto.StepProgressDTO{Index: 0, Status: "running", Title: "Fetching binary"}
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "installing", ActiveRun: &apidto.RunRecordDTO{
			Method: "_install", Steps: []apidto.StepProgressDTO{step},
		}},
		{Namespace: testNS, State: "installing", ActiveRun: &apidto.RunRecordDTO{
			Method: "_install", Steps: []apidto.StepProgressDTO{step},
		}},
		{Namespace: testNS, State: "ready", LastReturn: &apidto.ReturnDTO{Method: "_install", Outcome: "success"}},
	}}

	out, err := runCLI(t, f, "json", "install", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "success")
}

// A step reported as "success" mid-run (rather than only in a terminal
// outcome) renders the same as "succeeded"/"done".
func TestInstall_StepSuccessStatusRendersDone(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "installing", ActiveRun: &apidto.RunRecordDTO{
			Method: "_install",
			Steps:  []apidto.StepProgressDTO{{Index: 0, Status: "success", Title: "Fetching binary"}},
		}},
		{Namespace: testNS, State: "ready", LastReturn: &apidto.ReturnDTO{Method: "_install", Outcome: "success"}},
	}}

	out, err := runCLI(t, f, "json", "install", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "success")
}

// An invalid --output value is a usage error surfaced before any daemon
// call, the same as every other command that builds a runner.
func TestInstall_InvalidOutputFormatIsUsageError(t *testing.T) {
	f := &fakeDaemon{t: t}
	_, err := runCLI(t, f, "xml", "install", testNS)
	require.Error(t, err)
}

func TestInstall_DetachSkipsWait(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "json", "install", testNS, "--detach")
	require.NoError(t, err)
	assert.Contains(t, out, "started, not waiting")
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/install")
}

func TestRun_MapsToExecuteEndpoint(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "running", LastReturn: &apidto.ReturnDTO{Method: "_execute", Outcome: "success"}},
	}}

	_, err := runCLI(t, f, "json", "run", testNS)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/execute")
}

func TestStop_PostsStop(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "ready", LastReturn: &apidto.ReturnDTO{Method: "_stop", Outcome: "success"}},
	}}

	_, err := runCLI(t, f, "json", "stop", testNS)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/stop")
}

func TestUninstall_YesFlagSkipsConfirmation(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "absent", LastReturn: &apidto.ReturnDTO{Method: "_uninstall", Outcome: "success"}},
	}}

	_, err := runCLI(t, f, "json", "uninstall", testNS, "--yes")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/uninstall")
}

func TestUninstall_YesShorthandSkipsConfirmation(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "absent", LastReturn: &apidto.ReturnDTO{Method: "_uninstall", Outcome: "success"}},
	}}

	_, err := runCLI(t, f, "json", "uninstall", testNS, "-y")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/uninstall")
}

func TestUpdate_PostsUpdate(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "ready", LastReturn: &apidto.ReturnDTO{Method: "_update", Outcome: "success"}},
	}}

	_, err := runCLI(t, f, "json", "update", testNS)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/update")
}

// The row's previous run was an update too. The update bracket emits a
// runtime event before this update begins, still carrying the previous
// update's return: the command must wait for this run's own return.
func TestUpdate_PreviousUpdateReturnReplayed_WaitsForThisRun(t *testing.T) {
	previous := &apidto.ReturnDTO{
		Method: "_update", Outcome: "success", ExecutionID: "exec-previous",
		Steps: []apidto.StepProgressDTO{{Index: 0, Status: "completed", Title: "previous update"}},
	}
	current := &apidto.ReturnDTO{
		Method: "_update", Outcome: "success", ExecutionID: "exec-current",
		Steps: []apidto.StepProgressDTO{{Index: 0, Status: "completed", Title: "this update"}},
	}
	previousRaw, err := json.Marshal(apidto.ArrowRuntimeDTO{Namespace: testNS, State: "ready", LastReturn: previous})
	require.NoError(t, err)
	f := &fakeDaemon{t: t, runtimeDetail: string(previousRaw), wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "outdated", LastReturn: previous},
		{Namespace: testNS, State: "updating", LastReturn: previous, ActiveRun: &apidto.RunRecordDTO{
			Method: "_update",
			Steps:  []apidto.StepProgressDTO{{Index: 0, Status: "running", Title: "this update"}},
		}},
		{Namespace: testNS, State: "ready", LastReturn: current},
	}}

	out, err := runCLI(t, f, "json", "update", testNS)
	require.NoError(t, err)

	var run output.Run
	require.NoError(t, json.Unmarshal([]byte(out), &run))
	assert.Equal(t, "ready", run.State, "the stale outdated event of the previous update is not this run's end")
	require.Len(t, run.Steps, 1)
	assert.Equal(t, "this update", run.Steps[0].Title)
}

// A stream that only replays the previous run's return and closes never saw
// this run end: that is a failure, not a success.
func TestUpdate_OnlyThePreviousReturnArrives_Fails(t *testing.T) {
	previous := &apidto.ReturnDTO{Method: "_update", Outcome: "success", ExecutionID: "exec-previous"}
	previousRaw, err := json.Marshal(apidto.ArrowRuntimeDTO{Namespace: testNS, State: "ready", LastReturn: previous})
	require.NoError(t, err)
	f := &fakeDaemon{t: t, runtimeDetail: string(previousRaw), wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "outdated", LastReturn: previous},
	}}

	_, err = runCLI(t, f, "json", "update", testNS)
	require.Error(t, err)
}

// The previous return cannot be read: the command still waits for a return,
// exactly as before it knew to look for one.
func TestUpdate_PreviousReturnUnreadable_WaitsForAnyReturn(t *testing.T) {
	f := &fakeDaemon{t: t, runtimeDetail: `"not a runtime"`, wsScript: []apidto.ArrowRuntimeDTO{
		{Namespace: testNS, State: "ready", LastReturn: &apidto.ReturnDTO{Method: "_update", Outcome: "success"}},
	}}

	_, err := runCLI(t, f, "json", "update", testNS)
	require.NoError(t, err)
}

// A daemon answering 200 had nothing newer: no runtime event will follow, so
// the command must not wait on the stream. The stream here never speaks, so a
// command that waited would never return.
func TestUpdate_NoOp_DoesNotWaitForTheStream(t *testing.T) {
	testCases := []struct {
		name   string
		format string
		args   []string
		want   string
	}{
		{name: "json", format: "json", want: `"reason":"already up to date, nothing to do"`},
		{name: "yaml", format: "yaml", want: "reason: already up to date, nothing to do"},
		{name: "detached", format: "json", args: []string{"--detach"}, want: `"reason":"already up to date, nothing to do"`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hold := make(chan struct{})
			f := &fakeDaemon{t: t, mutationStatus: http.StatusOK, wsHold: hold}

			out, err := runCLI(t, f, tc.format, append([]string{"update", testNS}, tc.args...)...)
			close(hold)

			require.NoError(t, err)
			assert.Contains(t, strings.ReplaceAll(out, " ", ""), strings.ReplaceAll(tc.want, " ", ""))
			assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/update")
		})
	}
}

func TestUpdate_NoOpAlreadyUpToDate(t *testing.T) {
	f := &fakeDaemon{t: t, mutationStatus: http.StatusOK}

	out, err := runCLI(t, f, "json", "update", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "already up to date, nothing to do")
}

func TestActivate_StartedSaysTheDaemonIsRestarting(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "json", "activate", testNS)

	require.NoError(t, err)
	assert.Contains(t, out, "restarting to apply the update")
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "POST /v0/runtime/github.com%2Fuser%2Fapp/activate")
}

func TestActivate_NothingStagedIsANoOp(t *testing.T) {
	f := &fakeDaemon{t: t, mutationStatus: http.StatusOK}

	out, err := runCLI(t, f, "json", "activate", testNS)

	require.NoError(t, err)
	assert.Contains(t, out, "nothing staged to activate")
}

func TestActivate_NeverWaitsForARun(t *testing.T) {
	f := &fakeDaemon{t: t, wsHold: make(chan struct{})}

	out, err := runCLI(t, f, "json", "activate", testNS)

	require.NoError(t, err, "no run events follow an activation: the daemon restarts instead")
	assert.Contains(t, out, "restarting")
}

func TestActivate_InvalidNamespaceIsRefused(t *testing.T) {
	_, err := runCLI(t, &fakeDaemon{t: t}, "json", "activate", "not a namespace")

	require.Error(t, err)
}

func TestUninstall_NonTTYWithoutForceRefuses(t *testing.T) {
	_, err := runCLI(t, &fakeDaemon{t: t}, "json", "uninstall", testNS)
	require.Error(t, err)
}

// ─── confirm gate ────────────────────────────────────────────────────────────

func TestConfirm_NonTTYWithoutForceRefuses(t *testing.T) {
	_, err := runCLI(t, &fakeDaemon{t: t}, "json", "uninstall", testNS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
}

func TestConfirm_TTYAcceptsYes(t *testing.T) {
	// A 200 (not 202) response makes ExecuteMethod report the request as an
	// idempotent no-op, so the command completes without needing a scripted
	// WS terminal event — only the confirm gate itself is under test here.
	f := &fakeDaemon{t: t, mutationStatus: http.StatusOK}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	out, err := runRuntime(t, newSession(t, srv.URL, true), "json", strings.NewReader("y\n"),
		"uninstall", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "nothing to do")
}

func TestConfirm_TTYRejectsNo(t *testing.T) {
	f := &fakeDaemon{t: t}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	_, err := runRuntime(t, newSession(t, srv.URL, true), "json", strings.NewReader("n\n"),
		"uninstall", testNS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
}

// ─── data flag ───────────────────────────────────────────────────────────────

func TestData_ParsedIntoVariables(t *testing.T) {
	f := &fakeDaemon{t: t}
	_, err := runCLI(t, f, "json", "install", testNS, "--detach", "--data", "port=8080")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(f.recorded(), "\n"), "install")
}

func TestData_InvalidPairIsUsageError(t *testing.T) {
	_, err := runCLI(t, &fakeDaemon{t: t}, "json", "install", testNS, "--detach", "--data", "noequals")
	require.Error(t, err)
}

// ─── stream edge cases ────────────────────────────────────────────────────────

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
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	_, err := runRuntime(t, newSession(t, srv.URL, false), "json", nil, "install", testNS)
	assert.Error(t, err)
}

func TestInstall_StreamClosesWithoutTerminalEvent(t *testing.T) {
	f := &fakeDaemon{t: t} // empty wsScript: socket closes after 200ms
	_, err := runCLI(t, f, "json", "install", testNS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stream closed")
}

// ─── TTY lifecycle rendering ─────────────────────────────────────────────────

func TestInstall_TTYRendersModel(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: installScript()}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	out, err := runRuntime(t, newSession(t, srv.URL, true), "", strings.NewReader(""),
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
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	_, err := runRuntime(t, newSession(t, srv.URL, true), "json", strings.NewReader(""),
		"install", testNS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed")
}

func TestInstall_TTYStreamClosesWithoutTerminal(t *testing.T) {
	f := &fakeDaemon{t: t} // empty script: stream closes, model never done
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	_, err := runRuntime(t, newSession(t, srv.URL, true), "json", strings.NewReader(""),
		"install", testNS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stream closed before install completed")
}
