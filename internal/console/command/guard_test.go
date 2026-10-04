package command_test

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/console/command"
)

func findCommand(
	t *testing.T,
	root *cobra.Command,
	args ...string,
) *cobra.Command {
	t.Helper()

	found, _, err := root.Find(args)
	require.NoError(t, err)
	return found
}

func TestGuard_Check_RefusesACommandThatIsNotTheApprovedOne(t *testing.T) {
	root := command.NewRootForTest(executor())
	approved := findCommand(t, root, "list")
	other := findCommand(t, root, "health")

	err := command.CheckGuardForTest(approved, other)

	var denied *command.DeniedError
	require.True(t, errors.As(err, &denied))
	assert.Equal(t, "unknown command", denied.Reason)
}

func TestGuard_Check_RefusesARedirectingFlagThatWasSet(t *testing.T) {
	root := command.NewRootForTest(executor())
	list := findCommand(t, root, "list")
	require.NoError(t, list.ParseFlags([]string{"--server", "tcp://evil:1"}))

	err := command.CheckGuardForTest(list, list)

	var denied *command.DeniedError
	require.True(t, errors.As(err, &denied))
	assert.Contains(t, denied.Reason, "--server")
	assert.Equal(t, "quiver list", denied.Command)
}

func TestGuard_Check_RefusesADeniedFlagOfTheApprovedCommand(t *testing.T) {
	root := command.NewRootForTest(executor())
	status := findCommand(t, root, "status")
	require.NoError(t, status.ParseFlags([]string{"-w"}))

	err := command.CheckGuardForTest(status, status)

	var denied *command.DeniedError
	require.True(t, errors.As(err, &denied))
	assert.Contains(t, denied.Reason, "--watch")
}

func TestGuard_Check_AllowsTheApprovedCommandWithNoDeniedFlags(t *testing.T) {
	root := command.NewRootForTest(executor())
	list := findCommand(t, root, "list")
	require.NoError(t, list.ParseFlags([]string{"-o", "json"}))

	assert.NoError(t, command.CheckGuardForTest(list, list))
}

func consoleTree() *cobra.Command {
	root := &cobra.Command{Use: "q"}
	noop := func(*cobra.Command, []string) error { return nil }

	allowed := clierr.AllowInConsole(&cobra.Command{Use: "allowed", RunE: noop})
	hidden := clierr.AllowInConsole(&cobra.Command{Use: "hidden", RunE: noop, Hidden: true})
	group := clierr.AllowInConsole(&cobra.Command{Use: "group"})
	orphan := clierr.AllowInConsole(&cobra.Command{Use: "orphan", RunE: noop})
	closed := &cobra.Command{Use: "closed"}
	closed.AddCommand(orphan)
	root.AddCommand(allowed, hidden, group, closed)
	return root
}

func TestAuthorize_Resolution_HandlesTreesWithoutADispatchHandler(t *testing.T) {
	cases := map[string]string{
		"nonsense":      "unknown command",
		"hidden":        "not available",
		"group":         "needs a subcommand",
		"closed orphan": "not available",
	}
	for line, reason := range cases {
		_, err := command.Authorize(consoleTree(), []string{line})
		if line == "closed orphan" {
			_, err = command.Authorize(consoleTree(), []string{"closed", "orphan"})
		}
		var denied *command.DeniedError
		require.True(t, errors.As(err, &denied), line)
		assert.Contains(t, denied.Reason, reason, line)
	}

	found, err := command.Authorize(consoleTree(), []string{"allowed"})
	require.NoError(t, err)
	assert.Equal(t, "allowed", found.Name())
}

func TestAuthorize_Flags_LeavesParseErrorsToCobra(t *testing.T) {
	found, err := command.AuthorizeFlags(consoleTree(), []string{"allowed", "--unknown"})

	require.NoError(t, err)
	assert.Equal(t, "allowed", found.Name())
}

func TestAuthorize_Flags_PassesResolutionDenialsThrough(t *testing.T) {
	_, err := command.AuthorizeFlags(consoleTree(), []string{"hidden"})

	var denied *command.DeniedError
	assert.True(t, errors.As(err, &denied))
}
