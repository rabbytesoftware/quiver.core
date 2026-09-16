package commands_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
)

// assertMutation decodes out as a Mutation payload and checks it describes the
// operation the command was asked to perform.
func assertMutation(t *testing.T, out, action, subject string) {
	t.Helper()

	var m output.Mutation
	require.NoError(t, json.Unmarshal([]byte(out), &m), "mutation payload: %s", out)

	assert.Equal(t, output.Action(action), m.Action)
	assert.Equal(t, subject, m.Subject)

	_, err := time.Parse(time.RFC3339, m.At)
	assert.NoError(t, err, "at must be RFC3339, got %q", m.At)
}

// The context commands edit a local file. They must render the same payload as
// the catalog mutations without contacting a daemon, which is why they go
// through renderMutation rather than runMutation.
func TestContextMutations_EmitStructuredPayloadWithoutADaemon(t *testing.T) {
	cfg := t.TempDir() + "/config.yaml"

	out, err := runCLIConfig(t, cfg, "context", "add", "homelab",
		"--ctx-server", "tcp://10.0.0.5:40257")
	require.NoError(t, err)
	assertMutation(t, out, "add", "homelab")

	out, err = runCLIConfig(t, cfg, "context", "use", "homelab")
	require.NoError(t, err)
	assertMutation(t, out, "use", "homelab")

	out, err = runCLIConfig(t, cfg, "context", "remove", "homelab", "--force")
	require.NoError(t, err)
	assertMutation(t, out, "remove", "homelab")
}

func TestContextMutations_TableRendersOneLine(t *testing.T) {
	cfg := t.TempDir() + "/config.yaml"

	out, err := runCLIConfig(t, cfg, "context", "add", "homelab",
		"--ctx-server", "tcp://10.0.0.5:40257", "-o", "table")
	require.NoError(t, err)
	assert.Contains(t, out, "added homelab")

	out, err = runCLIConfig(t, cfg, "context", "use", "homelab", "-o", "table")
	require.NoError(t, err)
	assert.Contains(t, out, "switched to homelab")
}
