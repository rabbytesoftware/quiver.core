package ownership

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
)

func TestBundleTagOwner(t *testing.T) {
	bundle := filepath.Join("Applications", "Tool.app")
	testCases := []struct {
		name string
		tag  string
		want domain.Namespace
	}{
		{name: "tag naming this bundle is owned", tag: BundleTag(mocks.BareA, bundle), want: mocks.BareA},
		{name: "tag copied from another bundle is not owned", tag: BundleTag(mocks.BareA, filepath.Join("Applications", "Other.app"))},
		{name: "namespace-only tag is not owned", tag: mocks.BareA.String()},
		{name: "no tag is not owned", tag: ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, bundleTagOwner(tc.tag, bundle))
		})
	}
}

func TestBundles_Holder_NonDirectoryIsUnmanaged(t *testing.T) {
	b := NewBundles(&mocks.Tagger{})
	file := filepath.Join(t.TempDir(), "Tool.app")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	got, err := b.Holder(file)

	require.NoError(t, err)
	assert.Equal(t, Holder{Exists: true}, got)
}

func TestBundles_Holder_Absent(t *testing.T) {
	b := NewBundles(&mocks.Tagger{})

	got, err := b.Holder(filepath.Join(t.TempDir(), "missing.app"))

	require.NoError(t, err)
	assert.Equal(t, Holder{}, got)
}

func TestBundles_Holder_InspectError(t *testing.T) {
	b := NewBundles(&mocks.Tagger{})
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := b.Holder(filepath.Join(file, "Tool.app"))

	require.Error(t, err)
}

func TestBundles_Holder_TaggedBundleIsOwned(t *testing.T) {
	tagger := &mocks.Tagger{}
	b := NewBundles(tagger)
	bundle := filepath.Join(t.TempDir(), "Tool.app")
	mocks.WriteBundle(t, bundle, "x")
	require.NoError(t, b.Tag(bundle, mocks.BareA, bundle))

	got, err := b.Holder(bundle)

	require.NoError(t, err)
	assert.Equal(t, Holder{Exists: true, Namespace: mocks.BareA}, got)
}

func TestBundles_Owner_ReadErrorIsUnowned(t *testing.T) {
	tagger := &mocks.Tagger{ReadErr: errors.New("boom")}

	assert.Equal(t, domain.Namespace(""), NewBundles(tagger).Owner(t.TempDir()))
}

func TestBundles_Tag_WriteError(t *testing.T) {
	tagger := &mocks.Tagger{WriteErr: errors.New("boom")}

	require.ErrorIs(t, NewBundles(tagger).Tag(t.TempDir(), mocks.BareA, "Tool.app"), tagger.WriteErr)
}

func TestBundles_Enclosing(t *testing.T) {
	tagger := &mocks.Tagger{}
	b := NewBundles(tagger)
	apps := t.TempDir()
	tagged := filepath.Join(apps, "Tool.app")
	mocks.WriteBundle(t, tagged, "x")
	require.NoError(t, tagger.Write(tagged, BundleTag(mocks.BareA, tagged)))
	untagged := filepath.Join(apps, "Other"+platform.BundleExt)
	mocks.WriteBundle(t, untagged, "x")

	assert.Equal(t, mocks.BareA, b.Enclosing(filepath.Join(tagged, "Contents", "MacOS", "tool")))
	assert.Equal(t, domain.Namespace(""), b.Enclosing(filepath.Join(untagged, "Contents", "tool")))

	tagger.ReadErr = errors.New("boom")
	assert.Equal(t, domain.Namespace(""), b.Enclosing(filepath.Join(tagged, "Contents", "tool")))
}
