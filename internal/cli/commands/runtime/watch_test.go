package runtime_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
)

// The manual documents `quiver watch <ns>` as the raw live event stream, and
// points at it from Troubleshooting for a run that looks stuck.
func TestWatch_StreamsRuntimeEvents(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: installScript()}

	out, err := runCLI(t, f, "table", "watch", testNS)
	require.NoError(t, err)
	assert.Contains(t, out, "installing")
	assert.Contains(t, out, "ready")
	assert.Contains(t, out, "Fetching binary")
}

func TestWatch_RejectsInvalidNamespace(t *testing.T) {
	f := &fakeDaemon{t: t}

	_, err := runCLI(t, f, "json", "watch", "not-a-namespace")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid namespace")
}

// `quiver status <ns> -w` is documented as "keep watching live", so it streams
// the same events rather than returning one snapshot.
func TestStatus_WatchFlagStreams(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: installScript()}

	out, err := runCLI(t, f, "table", "status", testNS, "-w")
	require.NoError(t, err)
	assert.Contains(t, out, "installing")
	assert.Contains(t, out, "ready")
}

func TestStatus_WatchWithoutNamespaceIsUsageError(t *testing.T) {
	f := &fakeDaemon{t: t}

	_, err := runCLI(t, f, "json", "status", "-w")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs a namespace")
}

func TestWatch_JSONPayloadCarriesEvents(t *testing.T) {
	f := &fakeDaemon{t: t, wsScript: installScript()}

	out, err := runCLI(t, f, "json", "watch", testNS)
	require.NoError(t, err)

	var w output.Watch
	require.NoError(t, json.Unmarshal([]byte(out), &w), "watch payload: %s", out)
	assert.Equal(t, testNS, w.Subject)
	require.Len(t, w.Events, 3)
	assert.Equal(t, "installing", w.Events[0].State)
	assert.Equal(t, "ready", w.Events[2].State)
}

func TestWatch_UnreachableDaemonIsConnectionError(t *testing.T) {
	// Port 0 is never listening, so the dial fails without touching a daemon.
	out, err := runRuntime(t, newSession(t, "http://127.0.0.1:0", false), "json", nil, "watch", testNS)

	require.Error(t, err)
	assert.Empty(t, out)
	assert.Equal(t, client.ExitConnection, clierr.ExitCode(err))
}
