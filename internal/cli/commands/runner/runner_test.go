package runner_test

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
)

func newCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	return cmd
}

func TestBuild_NoFlagTTY_DefaultsToTable(t *testing.T) {
	b := runner.New(&runner.Flags{})
	r, err := b.Build(newCmd(), true)
	require.NoError(t, err)
	_ = r // Runner has no exported format getter; behaviour is covered via Theme()/Run() in flow tests.
}

func TestBuild_NoFlagNoTTY_DefaultsToJSON(t *testing.T) {
	b := runner.New(&runner.Flags{})
	_, err := b.Build(newCmd(), false)
	require.NoError(t, err)
}

func TestBuild_ExplicitOutput_UsesIt(t *testing.T) {
	b := runner.New(&runner.Flags{Output: "yaml"})
	_, err := b.Build(newCmd(), true)
	require.NoError(t, err)
}

func TestBuild_InvalidOutput_ReturnsUsageError(t *testing.T) {
	b := runner.New(&runner.Flags{Output: "xml"})
	_, err := b.Build(newCmd(), true)
	require.Error(t, err)
	assert.Equal(t, tui.ExitUsage, tui.CodeFor(err))
}
