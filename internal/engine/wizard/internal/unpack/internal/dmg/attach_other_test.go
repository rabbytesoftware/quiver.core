//go:build !darwin

package dmg_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/dmg"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func TestUnpack_UnsupportedOffDarwin(t *testing.T) {
	to := filepath.Join(t.TempDir(), "out")
	format := detectWith(t, dmg.New(mocks.TestMaxBytes, guard.HostRules{}))

	_, err := format.Unpack(context.Background(), models.Target{Dir: to})

	require.ErrorIs(t, err, models.ErrUnsupportedPlatform)
	entries, err := os.ReadDir(to)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
