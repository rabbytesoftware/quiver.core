package shelf

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/launch"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
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

func TestLaunchAllowed(t *testing.T) {
	l := models.Layout{Apps: []string{"/Applications", "/Users/me/Applications"}}
	wd := filepath.FromSlash("/ns/github.com/a/b@v1")
	in := func(rel string) string { return filepath.Join(wd, filepath.FromSlash(rel)) }

	testCases := []struct {
		name   string
		target string
		want   bool
	}{
		{"inside the workdir", in("bin/tool"), true},
		{"the workdir itself is inside", wd, true},
		{"outside the workdir", filepath.FromSlash("/usr/bin/tool"), false},
		{"dot dot escapes the workdir", filepath.Clean(in("../other/tool")), false},
		{"bundle directly in an applications folder", filepath.FromSlash("/Applications/Tool.app"), true},
		{"bundle extension is case-insensitive", filepath.FromSlash("/Applications/Tool.APP"), true},
		{"bundle nested under an applications folder", filepath.FromSlash("/Applications/Sub/Tool.app"), false},
		{"bundle outside every applications folder", filepath.FromSlash("/tmp/Tool.app"), false},
		{"plain executable in an applications folder", filepath.FromSlash("/Applications/tool"), false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, launchAllowed(l, wd, tc.target))
		})
	}
}

func TestShelf_Launchable_RejectsUnusableTargets(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	ctx := context.Background()
	record := func(target string) {
		require.NoError(t, launch.Record(wd, target))
	}

	record(wd)
	assert.False(t, f.shelf.Launchable(ctx, wd), "a directory cannot be started")

	plain := filepath.Join(wd, "plain")
	mocks.WriteFile(t, plain, "x", 0o644)
	record(plain)
	assert.False(t, f.shelf.Launchable(ctx, wd), "a file without an exec bit cannot be started")

	record(filepath.Join(wd, "gone"))
	assert.False(t, f.shelf.Launchable(ctx, wd), "a missing target cannot be started")

	record(filepath.Join(wd, "..", "..", "escape"))
	assert.False(t, f.shelf.Launchable(ctx, wd), "a target outside the workdir cannot be started")

	runnable := filepath.Join(wd, "runnable")
	mocks.WriteFile(t, runnable, "#!/bin/sh\n", 0o755)
	record(runnable)
	assert.True(t, f.shelf.Launchable(ctx, wd))
}

func TestShelf_Remove_DropsTheLaunchTarget(t *testing.T) {
	mocks.RequireUnixHost(t)
	f := newFixture(t, "linux")
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "Tool.AppImage"), "x", 0o755)
	ctx := context.Background()
	desktop := domain.Expose{Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "Tool.AppImage"}}}
	_, err := f.shelf.Apply(ctx, mocks.NsA, wd, desktop, domain.ArrowMedia{})
	require.NoError(t, err)
	require.True(t, f.shelf.Launchable(ctx, wd))

	require.NoError(t, f.shelf.Remove(ctx, wd))

	target, err := launch.Recorded(wd)
	require.NoError(t, err)
	assert.Empty(t, target)
}
