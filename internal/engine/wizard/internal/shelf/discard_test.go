package shelf

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

func TestShelf_Discard_EmptiesTheWorkdirAndKeepsIt(t *testing.T) {
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "app", "bin", "tool"), "x", 0o755)
	mocks.WriteFile(t, filepath.Join(wd, ".quiver-launch"), "x", 0o600)

	require.NoError(t, f.shelf.Discard(context.Background(), wd))

	entries, err := os.ReadDir(wd)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestShelf_Discard_RefusesADirectoryOutsideTheNamespaces(t *testing.T) {
	f := newFixture(t, "linux")
	outside := t.TempDir()
	mocks.WriteFile(t, filepath.Join(outside, "keep"), "x", 0o644)

	err := f.shelf.Discard(context.Background(), outside)

	assert.ErrorIs(t, err, ErrNotAWorkdir)
	assert.FileExists(t, filepath.Join(outside, "keep"))
}
