package invoke_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

type stubSession struct {
	client *client.Client
	err    error
}

func (s stubSession) Config() (*config.Config, error) { return nil, nil }
func (s stubSession) Client(context.Context, *cobra.Command) (*client.Client, error) {
	return s.client, s.err
}
func (s stubSession) Spinner(*cobra.Command, string, func() error) error { return nil }
func (s stubSession) IsTTY() bool                                        { return false }

func newCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	return cmd
}

func TestRunInstant_SessionError_ReturnsIt(t *testing.T) {
	sess := stubSession{err: errors.New("boom")}
	rb := runner.New(&runner.Flags{})

	err := invoke.RunInstant(
		sess, rb, newCmd(), "loading",
		func(*client.Client) (string, error) { return "", nil },
		func(string, theme.Theme) string { return "" },
	)
	require.Error(t, err)
	assert.Equal(t, "boom", err.Error())
}

func TestRunInstant_Success_RendersPayload(t *testing.T) {
	cli, err := client.New("http://unused")
	require.NoError(t, err)
	sess := stubSession{client: cli}
	rb := runner.New(&runner.Flags{Output: "json"})

	out := &bytes.Buffer{}
	cmd := newCmd()
	cmd.SetOut(out)

	err = invoke.RunInstant(
		sess, rb, cmd, "loading",
		func(*client.Client) (string, error) { return "hello", nil },
		func(s string, _ theme.Theme) string { return s },
	)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "hello")
}

func TestRenderInstant_Success_RendersPayload(t *testing.T) {
	rb := runner.New(&runner.Flags{Output: "json"})
	out := &bytes.Buffer{}
	cmd := newCmd()
	cmd.SetOut(out)

	err := invoke.RenderInstant(
		stubSession{}, rb, cmd, "loading",
		func() (string, error) { return "hi", nil },
		func(s string, _ theme.Theme) string { return s },
	)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "hi")
}

func TestRunMutation_Success_RendersMutation(t *testing.T) {
	cli, err := client.New("http://unused")
	require.NoError(t, err)
	sess := stubSession{client: cli}
	rb := runner.New(&runner.Flags{Output: "json"})

	out := &bytes.Buffer{}
	cmd := newCmd()
	cmd.SetOut(out)

	called := false
	err = invoke.RunMutation(
		sess, rb, cmd, output.ActionAdd, "github.com/u/r",
		func(*client.Client) error { called = true; return nil },
	)
	require.NoError(t, err)
	assert.True(t, called)
	assert.Contains(t, out.String(), "github.com/u/r")
}

func TestRenderMutation_DoFails_ReturnsError(t *testing.T) {
	rb := runner.New(&runner.Flags{Output: "json"})
	cmd := newCmd()

	err := invoke.RenderMutation(
		stubSession{}, rb, cmd, output.ActionUse, "ctx-name",
		func() error { return errors.New("nope") },
	)
	require.Error(t, err)
}

func TestRenderInstant_BuildFails_ReturnsIt(t *testing.T) {
	rb := runner.New(&runner.Flags{Output: "bogus"})

	err := invoke.RenderInstant(
		stubSession{}, rb, newCmd(), "loading",
		func() (string, error) { return "", nil },
		func(string, theme.Theme) string { return "" },
	)
	require.Error(t, err)
}

func TestRunMutation_SessionError_ReturnsIt(t *testing.T) {
	sess := stubSession{err: errors.New("boom")}
	rb := runner.New(&runner.Flags{})

	err := invoke.RunMutation(
		sess, rb, newCmd(), output.ActionAdd, "github.com/u/r",
		func(*client.Client) error { return nil },
	)
	require.Error(t, err)
	assert.Equal(t, "boom", err.Error())
}

func TestRenderMutation_BuildFails_ReturnsIt(t *testing.T) {
	rb := runner.New(&runner.Flags{Output: "bogus"})

	err := invoke.RenderMutation(
		stubSession{}, rb, newCmd(), output.ActionUse, "ctx-name",
		func() error { return nil },
	)
	require.Error(t, err)
}
