package commands_test

import (
	"encoding/json"
	"strings"
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

// Piped stdout defaults to json, the same rule every other command follows.
// The catalog mutations used to ignore --output entirely and print a fixed
// line, which is what this covers against regressing.
func TestArrowMutations_PipedEmitStructuredPayload(t *testing.T) {
	testCases := []struct {
		name   string
		args   []string
		action string
	}{
		{"add", []string{"arrow", "add", testNS}, "add"},
		{"remove", []string{"arrow", "remove", testNS, "-y"}, "remove"},
		{"refresh", []string{"arrow", "refresh", testNS}, "refresh"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runCLI(t, &fakeDaemon{t: t}, tc.args...)
			require.NoError(t, err)

			assertMutation(t, out, tc.action, testNS)
		})
	}
}

// -o yaml must describe the same mutation as -o json. yaml.v3 falls back to
// lowercased Go field names for any field missing a yaml tag, which would make
// the two formats disagree.
func TestArrowMutations_YAMLMatchesJSON(t *testing.T) {
	out, err := runCLI(t, &fakeDaemon{t: t}, "arrow", "add", testNS, "-o", "yaml")
	require.NoError(t, err)

	assert.Contains(t, out, "action: add")
	assert.Contains(t, out, "subject: "+testNS)
	assert.NotContains(t, out, "actionn")
}

// The table path keeps the human-readable line the commands have always
// printed, so a terminal user sees no change.
func TestArrowMutations_TableRendersOneLine(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want string
	}{
		{"add", []string{"arrow", "add", testNS, "-o", "table"}, "added " + testNS},
		{"remove", []string{"arrow", "remove", testNS, "-y", "-o", "table"}, "removed " + testNS},
		{"refresh", []string{"arrow", "refresh", testNS, "-o", "table"}, "refreshed " + testNS},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runCLI(t, &fakeDaemon{t: t}, tc.args...)
			require.NoError(t, err)

			assert.Contains(t, out, tc.want)
			assert.Equal(t, 1, strings.Count(strings.TrimSpace(out), "\n")+1,
				"a mutation renders one line, got %q", out)
		})
	}
}

// A failed mutation must not render a payload: the error is the result, and a
// payload on stdout would tell a script the opposite.
func TestArrowMutation_Failure_WritesNoPayload(t *testing.T) {
	f := &fakeDaemon{t: t, mutationStatus: 500}

	out, err := runCLI(t, f, "arrow", "add", testNS)
	require.Error(t, err)

	assert.NotContains(t, out, `"action"`)
}

func TestCollectionMutations_EmitStructuredPayload(t *testing.T) {
	const colNS = "github.com/user/col"

	testCases := []struct {
		name   string
		args   []string
		action string
	}{
		{"follow", []string{"collection", "follow", colNS}, "follow"},
		{"unfollow", []string{"collection", "unfollow", colNS, "-y"}, "unfollow"},
		{"update", []string{"collection", "update", colNS}, "update"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runCLI(t, &fakeDaemon{t: t}, tc.args...)
			require.NoError(t, err)

			assertMutation(t, out, tc.action, colNS)
		})
	}
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
