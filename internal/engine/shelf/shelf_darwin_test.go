//go:build darwin

package shelf

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
)

func TestShelf_Apply_DarwinMovesBundleAndSetsXattr(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin, withTagger(ownership.NewTagger()))
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteBundle(t, filepath.Join(wd, "Tool.app"), "v1")
	expose := domain.Expose{Desktop: []domain.ExposeEntry{{Name: "Tool", Path: domain.ExposeAuto}}}
	dest := filepath.Join(f.Apps[0], "Tool.app")

	got, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Empty(t, got.Refused)
	assert.Equal(t, []AppliedEntry{{Kind: domain.ExposeKindDesktop, Name: "Tool", Target: filepath.Join(wd, "Tool.app"), Location: dest}}, got.Entries)
	assert.NoDirExists(t, filepath.Join(wd, "Tool.app"))
	tag, err := ownership.NewTagger().Read(dest)
	require.NoError(t, err)
	assert.Equal(t, ownership.BundleTag(mocks.BareA, dest), tag)

	again, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)
	assert.Equal(t, got, again)

	require.NoError(t, f.shelf.Remove(context.Background(), mocks.NsA))
	assert.NoDirExists(t, dest)
}

func TestShelf_Remove_DarwinKeepsCopiedBundle(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin, withTagger(ownership.NewTagger()))
	wd := f.Workdir(t, mocks.NsA)
	mocks.WriteBundle(t, filepath.Join(wd, "Tool.app"), "v1")
	expose := domain.Expose{Desktop: []domain.ExposeEntry{{Name: "Tool", Path: domain.ExposeAuto}}}
	dest := filepath.Join(f.Apps[0], "Tool.app")
	_, err := f.shelf.Apply(context.Background(), mocks.NsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)
	copied := filepath.Join(f.Apps[0], "Tool copy.app")
	mocks.WriteBundle(t, copied, "user copy")
	tag, err := ownership.NewTagger().Read(dest)
	require.NoError(t, err)
	require.NoError(t, ownership.NewTagger().Write(copied, tag))

	require.NoError(t, f.shelf.Remove(context.Background(), mocks.NsA))

	assert.NoDirExists(t, dest)
	assert.Equal(t, "user copy", mocks.ReadBundleVersion(t, copied))
}
