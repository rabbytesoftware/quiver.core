package shelf

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

func TestShelf_Launchable_FollowsTheDesktopEntryApplyPlaced(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "Tool.AppImage"), "x", 0o755)
	mocks.WriteFile(t, filepath.Join(wd, "cli"), "x", 0o755)
	ctx := context.Background()

	_, err := f.shelf.Apply(ctx, mocks.NsA, wd, cliExpose("cli", "cli"), domain.ArrowMedia{})
	require.NoError(t, err)
	assert.False(t, f.shelf.Launchable(ctx, wd), "a CLI-only arrow has nothing to open")
	assert.ErrorIs(t, f.shelf.Launch(ctx, wd), ErrNotLaunchable)

	desktop := domain.Expose{Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "Tool.AppImage"}}}
	_, err = f.shelf.Apply(ctx, mocks.NsA, wd, desktop, domain.ArrowMedia{})
	require.NoError(t, err)
	assert.True(t, f.shelf.Launchable(ctx, wd))

	_, err = f.shelf.Apply(ctx, mocks.NsA, wd, cliExpose("cli", "cli"), domain.ArrowMedia{})
	require.NoError(t, err)
	assert.False(t, f.shelf.Launchable(ctx, wd), "an update that drops the desktop entry drops the target")
}

func TestShelf_Launch_RefusesAWorkdirOutsideTheNamespaces(t *testing.T) {
	f := newFixture(t, "linux")

	err := f.shelf.Launch(context.Background(), t.TempDir())

	assert.ErrorIs(t, err, ErrNotAWorkdir)
}
