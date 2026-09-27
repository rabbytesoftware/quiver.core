package shelf

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecCommander_Run(t *testing.T) {
	out, err := defaultCommander.Run(context.Background(), "go", "env", "GOOS")

	require.NoError(t, err)
	assert.Equal(t, runtime.GOOS, strings.TrimSpace(string(out)))
}

func TestExecCommander_Run_MissingBinary(t *testing.T) {
	_, err := defaultCommander.Run(context.Background(), "quiver-shelf-no-such-binary")

	assert.Error(t, err)
}

func TestExecCommander_RunWithEnv_PassesEnvironment(t *testing.T) {
	out, err := defaultCommander.RunWithEnv(context.Background(), []string{"GOFLAGS=-mod=mod"}, "go", "env", "GOFLAGS")

	require.NoError(t, err)
	assert.Equal(t, "-mod=mod", strings.TrimSpace(string(out)))
}
