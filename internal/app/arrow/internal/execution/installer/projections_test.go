package installer

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/require"
)

func TestInstaller_New_ReturnsNil(t *testing.T) {
	axArrow := buildAsynxArrow(t)
	axRuntime := buildAsynxRuntime(t)

	inst := New(
		axArrow,
		axRuntime,
		&mocks.Vault{},
		&mockRunner{},
	)

	require.NotNil(t, inst)
}
