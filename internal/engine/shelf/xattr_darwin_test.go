//go:build darwin

package shelf

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestXattrTagger_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	untagged, err := defaultTagger.read(dir)
	require.NoError(t, err)
	require.NoError(t, defaultTagger.write(dir, bundleTag(bareA, dir)))
	tagged, err := defaultTagger.read(dir)
	require.NoError(t, err)

	assert.Empty(t, untagged)
	assert.Equal(t, bundleTag(bareA, dir), tagged)
}

func TestXattrTagger_MissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")

	_, err := defaultTagger.read(missing)
	assert.Error(t, err)
	assert.Error(t, defaultTagger.write(missing, bundleTag(bareA, missing)))
}

func TestShelf_Apply_DarwinMovesBundleAndSetsXattr(t *testing.T) {
	f := newFixture(t, goosDarwin, withTagger(defaultTagger))
	wd := f.workdir(t, nsA)
	writeBundle(t, filepath.Join(wd, "Tool.app"), "v1")
	expose := domain.Expose{Desktop: []domain.ExposeEntry{{Name: "Tool", Path: domain.ExposeAuto}}}
	dest := filepath.Join(f.apps[0], "Tool.app")

	got, err := f.shelf.Apply(context.Background(), nsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)

	assert.Empty(t, got.Refused)
	assert.Equal(t, []AppliedEntry{{Kind: domain.ExposeKindDesktop, Name: "Tool", Target: filepath.Join(wd, "Tool.app"), Location: dest}}, got.Entries)
	assert.NoDirExists(t, filepath.Join(wd, "Tool.app"))
	tag, err := defaultTagger.read(dest)
	require.NoError(t, err)
	assert.Equal(t, bundleTag(bareA, dest), tag)

	again, err := f.shelf.Apply(context.Background(), nsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)
	assert.Equal(t, got, again)

	require.NoError(t, f.shelf.Remove(context.Background(), nsA))
	assert.NoDirExists(t, dest)
}

func TestShelf_Remove_DarwinKeepsCopiedBundle(t *testing.T) {
	f := newFixture(t, goosDarwin, withTagger(defaultTagger))
	wd := f.workdir(t, nsA)
	writeBundle(t, filepath.Join(wd, "Tool.app"), "v1")
	expose := domain.Expose{Desktop: []domain.ExposeEntry{{Name: "Tool", Path: domain.ExposeAuto}}}
	dest := filepath.Join(f.apps[0], "Tool.app")
	_, err := f.shelf.Apply(context.Background(), nsA, wd, expose, domain.ArrowMedia{})
	require.NoError(t, err)
	copied := filepath.Join(f.apps[0], "Tool copy.app")
	writeBundle(t, copied, "user copy")
	tag, err := defaultTagger.read(dest)
	require.NoError(t, err)
	require.NoError(t, defaultTagger.write(copied, tag))

	require.NoError(t, f.shelf.Remove(context.Background(), nsA))

	assert.NoDirExists(t, dest)
	assert.Equal(t, "user copy", readBundleVersion(t, copied))
}
