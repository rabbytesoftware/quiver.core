package quivercore_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	quivercore "github.com/rabbytesoftware/quiver.core"
)

func TestArrowManifest_Embedded_NotEmpty(t *testing.T) {
	require.NotEmpty(t, quivercore.ArrowManifest)
}
