//go:build darwin

package ownership

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/mocks"
)

func TestXattrTagger_RoundTrip(t *testing.T) {
	tagger := NewTagger()
	dir := t.TempDir()

	untagged, err := tagger.Read(dir)
	require.NoError(t, err)
	require.NoError(t, tagger.Write(dir, BundleTag(mocks.BareA, "/wd", dir)))
	tagged, err := tagger.Read(dir)
	require.NoError(t, err)

	assert.Empty(t, untagged)
	assert.Equal(t, BundleTag(mocks.BareA, "/wd", dir), tagged)
}

func TestXattrTagger_MissingPath(t *testing.T) {
	tagger := NewTagger()
	missing := filepath.Join(t.TempDir(), "missing")

	_, err := tagger.Read(missing)
	assert.Error(t, err)
	assert.Error(t, tagger.Write(missing, BundleTag(mocks.BareA, "/wd", missing)))
}
