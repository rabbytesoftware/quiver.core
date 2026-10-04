package command_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/console/command"
)

func rootWith(
	run func(cmd *cobra.Command) error,
) *cobra.Command {
	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	root.RunE = func(cmd *cobra.Command, _ []string) error { return run(cmd) }
	root.SetArgs([]string{})
	return root
}

func TestInvocation_RecoversAPanicIntoAFailedResult(t *testing.T) {
	root := rootWith(func(*cobra.Command) error { panic("boom") })

	result := command.NewInvocationForTest(root).Run(context.Background(), io.Discard, io.Discard)

	assert.Equal(t, command.Result{Code: 1, Err: "internal error"}, result)
}

func TestInvocation_ReturnsWhenTheContextIsDoneEvenIfTheCommandIgnoresIt(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	root := rootWith(func(*cobra.Command) error {
		<-release
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := command.NewInvocationForTest(root).Run(ctx, io.Discard, io.Discard)

	assert.Less(t, time.Since(start), 2*time.Second)
	assert.Equal(t, 1, result.Code)
	assert.Contains(t, result.Err, "deadline")
}

func TestInvocation_PassesTheContextToTheCommand(t *testing.T) {
	seen := make(chan context.Context, 1)
	root := rootWith(func(cmd *cobra.Command) error {
		seen <- cmd.Context()
		return nil
	})
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "v")

	command.NewInvocationForTest(root).Run(ctx, io.Discard, io.Discard)

	assert.Equal(t, "v", (<-seen).Value(key{}))
}

func TestInvocation_RoutesOutputToTheGivenWriters(t *testing.T) {
	root := rootWith(func(cmd *cobra.Command) error {
		_, _ = cmd.OutOrStdout().Write([]byte("out"))
		_, _ = cmd.ErrOrStderr().Write([]byte("err"))
		return nil
	})
	var stdout, stderr bytes.Buffer

	command.NewInvocationForTest(root).Run(context.Background(), &stdout, &stderr)

	assert.Equal(t, "out", stdout.String())
	assert.Equal(t, "err", stderr.String())
}

func TestInvocation_MapsACommandErrorToAResult(t *testing.T) {
	root := rootWith(func(*cobra.Command) error { return errors.New("it failed") })

	result := command.NewInvocationForTest(root).Run(context.Background(), io.Discard, io.Discard)

	assert.Equal(t, 1, result.Code)
	assert.Equal(t, "it failed", result.Err)
}

func TestNewRoot_GivesCommandsAnEmptyStdin(t *testing.T) {
	root := command.NewRootForTest(executor())

	data, err := io.ReadAll(root.InOrStdin())

	require.NoError(t, err)
	assert.Empty(t, data)
}
