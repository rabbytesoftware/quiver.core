package host_test

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/host"
)

func TestExecCommander_Run_ReturnsOutput(t *testing.T) {
	out, err := host.NewCommander().Run(context.Background(), "go", "env", "GOOS")

	require.NoError(t, err)
	assert.Equal(t, runtime.GOOS, strings.TrimSpace(string(out)))
}

func TestExecCommander_Run_MissingBinary(t *testing.T) {
	_, err := host.NewCommander().Run(context.Background(), "quiver-shelf-no-such-binary")

	assert.Error(t, err)
}
