package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCommand_HasDaemonSubcommand(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"daemon"})
	require.NoError(t, err)
	assert.Equal(t, "daemon", cmd.Name())
}

func TestDaemonCommand_HasHostFlag(t *testing.T) {
	cmd := newDaemonCmd()
	assert.NotNil(t, cmd.Flags().Lookup("host"))
}

func TestDaemonCommand_HostFlag_AcceptsURIFormats(t *testing.T) {
	cmd := newDaemonCmd()
	flag := cmd.Flags().Lookup("host")
	assert.NotNil(t, flag)
	assert.Equal(t, "", flag.DefValue)
}

func TestDaemonCommand_DoesNotExposeUpdateAttemptTokenFlag(t *testing.T) {
	cmd := newDaemonCmd()
	assert.Nil(t, cmd.Flags().Lookup("update-attempt-token"))
}

func TestResolveBuildInfoWith_ValidExecutableReturnsIdentity(t *testing.T) {
	info, err := resolveBuildInfoWith(
		context.Background(),
		"attempt",
		func() (string, error) { return "/bin/quiver", nil },
		func(_ context.Context, path string) (string, error) {
			assert.Equal(t, "/bin/quiver", path)
			return "digest", nil
		},
	)

	require.NoError(t, err)
	assert.Equal(t, version, info.Version)
	assert.Equal(t, buildID, info.BuildID)
	assert.Equal(t, "digest", info.Digest)
	assert.Equal(t, "attempt", info.AttemptToken)
}

func TestResolveBuildInfo_RunningTestBinaryReturnsDigest(t *testing.T) {
	t.Setenv(updateAttemptTokenEnv, "attempt")
	info, err := resolveBuildInfo(context.Background())

	require.NoError(t, err)
	assert.Len(t, info.Digest, 64)
	assert.Equal(t, "attempt", info.AttemptToken)
}

func TestResolveBuildInfoWith_ExecutableFailureReturnsError(t *testing.T) {
	wantErr := errors.New("executable failed")

	info, err := resolveBuildInfoWith(
		context.Background(),
		"attempt",
		func() (string, error) { return "", wantErr },
		func(context.Context, string) (string, error) { return "", nil },
	)

	require.ErrorIs(t, err, wantErr)
	assert.Empty(t, info)
}

func TestResolveBuildInfoWith_DigestFailureReturnsError(t *testing.T) {
	wantErr := errors.New("digest failed")

	info, err := resolveBuildInfoWith(
		context.Background(),
		"attempt",
		func() (string, error) { return "/bin/quiver", nil },
		func(context.Context, string) (string, error) { return "", wantErr },
	)

	require.ErrorIs(t, err, wantErr)
	assert.Empty(t, info)
}
