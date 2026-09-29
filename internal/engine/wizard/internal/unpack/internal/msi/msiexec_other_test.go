//go:build !windows

package msi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func TestUnpack_UnsupportedOffWindows(t *testing.T) {
	src, err := os.Open(writePackage(t, "setup.msi", oleFile()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	format, ok, err := New(mocks.TestMaxBytes, guard.NameRules{})(src, 0)
	require.NoError(t, err)
	require.True(t, ok)
	to := filepath.Join(t.TempDir(), "tool")

	_, err = format.Unpack(context.Background(), models.Target{Dir: to})

	require.ErrorIs(t, err, models.ErrUnsupportedPlatform)
	entries, err := os.ReadDir(to)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
