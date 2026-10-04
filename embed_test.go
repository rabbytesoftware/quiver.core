package quivercore_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	quivercore "github.com/rabbytesoftware/quiver.core"
)

func TestSelfManifest_IsTheRepoRootARROWmd(t *testing.T) {
	onDisk, err := os.ReadFile("ARROW.md")
	require.NoError(t, err)
	require.Equal(t, string(onDisk), string(quivercore.SelfManifest()))
}
