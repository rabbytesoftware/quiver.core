package runtime_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPs_ShowsRunning(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "table", "ps")
	require.NoError(t, err)
	assert.Contains(t, out, testNS)
	assert.Contains(t, out, "running")
	assert.NotContains(t, out, "github.com/user/idle")
}

func TestPs_AllIncludesIdle(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "table", "ps", "--all")
	require.NoError(t, err)
	assert.Contains(t, out, "github.com/user/idle")
}

func TestStatus_SingleArrow(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "table", "status", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "running")
	assert.Contains(t, out, "42")
}

func TestStatus_AllArrows(t *testing.T) {
	f := &fakeDaemon{t: t}

	out, err := runCLI(t, f, "table", "status")
	require.NoError(t, err)
	assert.Contains(t, out, testNS)
	assert.Contains(t, out, "github.com/user/idle")
}

// A run that has already finished is the case the manual points users at:
// "if a background install failed while you were away, this is where you find
// out what went wrong and at which step". The steps live on last_return, not
// on active_run, which is nil by then.
func TestStatus_ShowsFailedStepOfCompletedRun(t *testing.T) {
	f := &fakeDaemon{t: t, runtimeDetail: `{"namespace":"` + testNS + `","state":"absent",` +
		`"last_return":{"method":"_install","outcome":"failure","steps":[` +
		`{"index":1,"status":"completed","title":"Resolve dependencies","type":"noop"},` +
		`{"index":2,"status":"failed","title":"Downloading FFmpeg build","type":"fetch","error":"HTTP 404"}]}}`}

	out, err := runCLI(t, f, "table", "status", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "Downloading FFmpeg build")
	assert.Contains(t, out, "HTTP 404")
}

// An arrow that has never run has neither an active run nor a last return, so
// there is no step list to show and status must not invent one.
func TestStatus_NoStepsWhenNothingHasRun(t *testing.T) {
	f := &fakeDaemon{t: t, runtimeDetail: `{"namespace":"` + testNS + `","state":"absent"}`}

	out, err := runCLI(t, f, "table", "status", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "absent")
	assert.NotContains(t, out, "STEPS")
}

// An active run reported without a method name (e.g. right as it starts)
// must not render an empty "Method" field.
func TestStatus_ActiveRunWithoutMethodOmitsMethodField(t *testing.T) {
	f := &fakeDaemon{t: t, runtimeDetail: `{"namespace":"` + testNS + `","state":"installing","active_run":{"method":""}}`}

	out, err := runCLI(t, f, "table", "status", testNS)
	require.NoError(t, err)
	assert.NotContains(t, out, "Method")
}

func TestStatus_TableShowsLastReturn(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"namespace":"` + testNS +
			`","state":"ready","last_return":{"method":"_install","outcome":"success"}}}`))
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	out, err := runRuntime(t, newSession(t, srv.URL, false), "table", nil, "status", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "_install")
	assert.Contains(t, out, "success")
}
