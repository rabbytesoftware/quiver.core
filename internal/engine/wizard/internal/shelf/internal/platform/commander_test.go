package platform_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

func TestExecCommander_Run_PassesEnvironment(t *testing.T) {
	out, err := platform.NewCommander().Run(context.Background(), []string{"GOFLAGS=-mod=mod"}, "go", "env", "GOFLAGS")

	require.NoError(t, err)
	assert.Equal(t, "-mod=mod", strings.TrimSpace(string(out)))
}

func TestExecCommander_Run_MissingBinary(t *testing.T) {
	_, err := platform.NewCommander().Run(context.Background(), nil, "quiver-shelf-no-such-binary")

	assert.Error(t, err)
}
