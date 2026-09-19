package commands_test

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands"
)

func TestAttach_RegistersEveryTopLevelCommand(t *testing.T) {
	root := &cobra.Command{Use: "quiver"}
	root.SetOut(&bytes.Buffer{})
	commands.New(commands.Deps{IsTTY: func() bool { return false }}).Attach(root)

	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}

	for _, want := range []string{
		"install", "run", "stop", "uninstall", "update",
		"ps", "status", "watch",
		"list", "search", "info", "methods",
		"arrow", "collection", "context", "auth",
		"health", "version",
	} {
		assert.True(t, names[want], "missing top-level command %q", want)
	}
}

func TestExitCode_DelegatesToClierr(t *testing.T) {
	assert.Equal(t, 0, commands.ExitCode(nil))
}

func TestIsLifecycle_DelegatesToClierr(t *testing.T) {
	assert.False(t, commands.IsLifecycle(nil))
}

func TestIsActiveState_DelegatesToRuntime(t *testing.T) {
	assert.True(t, commands.IsActiveState("running"))
	assert.False(t, commands.IsActiveState("absent"))
}

func TestDispatch_NoArgs_ShowsHelp(t *testing.T) {
	root := &cobra.Command{Use: "quiver"}
	out := &bytes.Buffer{}
	root.SetOut(out)
	commands.New(commands.Deps{IsTTY: func() bool { return false }}).Attach(root)
	root.SetArgs([]string{})
	require.NoError(t, root.Execute())
	assert.Contains(t, out.String(), "Usage:")
}
