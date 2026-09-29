//go:build !darwin

package dmg_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/dmg"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func TestExtractDmg_Unsupported(t *testing.T) {
	dir := t.TempDir()
	image := mocks.WriteFile(t, filepath.Join(dir, "app.dmg"), []byte("image"))
	to := filepath.Join(dir, "out")

	g, err := guard.Open(context.Background(), to, mocks.TestMaxBytes)
	require.NoError(t, err)
	defer g.Close()

	err = dmg.Extract(context.Background(), image, g)

	require.Error(t, err)
	assert.Empty(t, g.TopLevel())
}
