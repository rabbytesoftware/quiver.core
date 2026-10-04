package clierr_test

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
)

func TestExitCode_Nil_ReturnsOK(t *testing.T) {
	assert.Equal(t, 0, clierr.ExitCode(nil))
}

func TestExitCode_TuiUsage_ReturnsExitUsage(t *testing.T) {
	assert.Equal(t, tui.ExitUsage, clierr.ExitCode(tui.Usage("bad flag")))
}

func TestExitCode_ClientConnError_ReturnsExitConnection(t *testing.T) {
	err := &client.ConnError{Server: "unix:///x", Err: assert.AnError}
	assert.Equal(t, client.ExitConnection, clierr.ExitCode(err))
}

func TestValidNS_Empty_ReturnsUsageError(t *testing.T) {
	err := clierr.ValidNS("")
	require.Error(t, err)
	assert.Equal(t, tui.ExitUsage, tui.CodeFor(err))
}

func TestValidNS_Valid_ReturnsNil(t *testing.T) {
	assert.NoError(t, clierr.ValidNS("github.com/user/repo"))
}

func TestParseData_Empty_ReturnsNil(t *testing.T) {
	got, err := clierr.ParseData(nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestParseData_ValidPairs_ReturnsMap(t *testing.T) {
	got, err := clierr.ParseData([]string{"a=1", "b=2"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "1", "b": "2"}, got)
}

func TestParseData_MissingEquals_ReturnsUsageError(t *testing.T) {
	_, err := clierr.ParseData([]string{"noequals"})
	require.Error(t, err)
	assert.Equal(t, tui.ExitUsage, tui.CodeFor(err))
}

func TestConfirm_Yes_SkipsPrompt(t *testing.T) {
	cmd := &cobra.Command{}
	assert.NoError(t, clierr.Confirm(cmd, false, true, "remove x"))
}

func TestConfirm_NonInteractiveNoYes_ReturnsUsageError(t *testing.T) {
	cmd := &cobra.Command{}
	err := clierr.Confirm(cmd, false, false, "remove x")
	require.Error(t, err)
	assert.Equal(t, tui.ExitUsage, tui.CodeFor(err))
}

func TestIsLifecycle_Annotated_ReturnsTrue(t *testing.T) {
	cmd := &cobra.Command{Annotations: map[string]string{clierr.AnnotationLifecycle: "true"}}
	assert.True(t, clierr.IsLifecycle(cmd))
}

func TestIsLifecycle_Nil_ReturnsFalse(t *testing.T) {
	assert.False(t, clierr.IsLifecycle(nil))
}

func TestConfirm_InteractiveYes_ReturnsNil(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("yes\n"))
	assert.NoError(t, clierr.Confirm(cmd, true, false, "remove x"))
}

func TestConfirm_InteractiveY_ReturnsNil(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("y\n"))
	assert.NoError(t, clierr.Confirm(cmd, true, false, "remove x"))
}

func TestConfirm_InteractiveNo_ReturnsUsageError(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("no\n"))
	err := clierr.Confirm(cmd, true, false, "remove x")
	require.Error(t, err)
	assert.Equal(t, tui.ExitUsage, tui.CodeFor(err))
}

func TestConfirm_InteractiveEmpty_ReturnsUsageError(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("\n"))
	err := clierr.Confirm(cmd, true, false, "remove x")
	require.Error(t, err)
	assert.Equal(t, tui.ExitUsage, tui.CodeFor(err))
}

func TestAllowInConsole_MarksOnlyThatCommand(t *testing.T) {
	parent := &cobra.Command{Use: "parent"}
	child := &cobra.Command{Use: "child"}
	parent.AddCommand(child)

	got := clierr.AllowInConsole(child)

	assert.Same(t, child, got)
	assert.True(t, clierr.IsConsole(child))
	assert.False(t, clierr.IsConsole(parent))
}

func TestAllowInConsole_KeepsExistingAnnotations(t *testing.T) {
	cmd := &cobra.Command{Use: "x", Annotations: map[string]string{clierr.AnnotationLifecycle: "true"}}

	clierr.AllowInConsole(cmd)

	assert.True(t, clierr.IsLifecycle(cmd))
	assert.True(t, clierr.IsConsole(cmd))
}

func TestIsConsole_NilAndUnmarkedAreFalse(t *testing.T) {
	assert.False(t, clierr.IsConsole(nil))
	assert.False(t, clierr.IsConsole(&cobra.Command{Use: "x"}))
	assert.False(t, clierr.IsConsole(&cobra.Command{Use: "x", Annotations: map[string]string{clierr.AnnotationConsole: "yes"}}))
}

func TestDenyInConsoleFlags_RoundTrips(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}

	clierr.DenyInConsoleFlags(cmd, "watch", "file")

	assert.Equal(t, []string{"watch", "file"}, clierr.ConsoleDeniedFlags(cmd))
	assert.Nil(t, clierr.ConsoleDeniedFlags(&cobra.Command{Use: "y"}))
	assert.Nil(t, clierr.ConsoleDeniedFlags(nil))
}
