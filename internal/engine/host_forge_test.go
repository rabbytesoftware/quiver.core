package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
)

type stubProvider struct{}

func (stubProvider) Host() string { return "stub.test" }

func (stubProvider) CanSearch() bool { return false }

func (stubProvider) Search(
	_ context.Context,
	_ provider.SearchRequest,
) ([]provider.Candidate, error) {
	return nil, nil
}

func (stubProvider) LatestRelease(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	return "", nil
}

func (stubProvider) RawFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "https://stub.test/file", nil
}

func (stubProvider) DefaultBranches() []string { return []string{"main"} }

type stubForgeProvider struct {
	stubProvider
	releaseErr error
	pageErr    error
	rawErr     error
}

func (s stubForgeProvider) ReleaseAssets(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) ([]provider.Asset, error) {
	if s.releaseErr != nil {
		return nil, s.releaseErr
	}
	return []provider.Asset{{Name: "a", URL: "u", Size: 1, Digest: "d"}}, nil
}

func (s stubForgeProvider) RepoPage(
	_ context.Context,
	_ domain.Namespace,
) (provider.RepoPage, error) {
	if s.pageErr != nil {
		return provider.RepoPage{}, s.pageErr
	}
	return provider.RepoPage{Description: "desc", OwnerIsOrg: true}, nil
}

func (s stubForgeProvider) RawFile(
	_ context.Context,
	_ domain.Namespace,
	_ string,
	_ string,
) ([]byte, error) {
	if s.rawErr != nil {
		return nil, s.rawErr
	}
	return []byte("bytes"), nil
}

func TestAdaptHost_NonForgeProvider_ReturnsItUnchanged(t *testing.T) {
	host := adaptHost(stubProvider{})
	_, ok := host.(manifold.Forge)
	assert.False(t, ok)
}

func TestAdaptHost_ForgeProvider_SatisfiesManifoldForge(t *testing.T) {
	host := adaptHost(stubForgeProvider{})
	_, ok := host.(manifold.Forge)
	assert.True(t, ok)
}

func TestForgeHost_StillSatisfiesHost(t *testing.T) {
	host := adaptHost(stubForgeProvider{})
	assert.Equal(t, []string{"main"}, host.DefaultBranches())
}

func TestForgeHost_ReleaseAssets_TranslatesAssets(t *testing.T) {
	host := adaptHost(stubForgeProvider{}).(manifold.Forge)

	assets, err := host.ReleaseAssets(context.Background(), domain.Namespace("stub.test/u/r"), "v1")
	require.NoError(t, err)
	assert.Equal(t, []manifold.Asset{{Name: "a", URL: "u", Size: 1, Digest: "d"}}, assets)
}

func TestForgeHost_RepoPage_TranslatesPage(t *testing.T) {
	host := adaptHost(stubForgeProvider{}).(manifold.Forge)

	page, err := host.RepoPage(context.Background(), domain.Namespace("stub.test/u/r"))
	require.NoError(t, err)
	assert.Equal(t, manifold.RepoPage{Description: "desc", OwnerIsOrg: true}, page)
}

func TestForgeHost_RawFile_ReturnsBody(t *testing.T) {
	host := adaptHost(stubForgeProvider{}).(manifold.Forge)

	body, err := host.RawFile(context.Background(), domain.Namespace("stub.test/u/r"), "main", "ARROW.md")
	require.NoError(t, err)
	assert.Equal(t, []byte("bytes"), body)
}

func TestForgeHost_ReleaseAssets_TranslatesReleaseNotFound(t *testing.T) {
	host := adaptHost(stubForgeProvider{releaseErr: provider.ErrReleaseNotFound}).(manifold.Forge)

	_, err := host.ReleaseAssets(context.Background(), domain.Namespace("stub.test/u/r"), "missing")
	assert.ErrorIs(t, err, manifold.ErrReleaseNotFound)
}

func TestForgeHost_RepoPage_TranslatesUnexpectedPage(t *testing.T) {
	host := adaptHost(stubForgeProvider{pageErr: provider.ErrUnexpectedPage}).(manifold.Forge)

	_, err := host.RepoPage(context.Background(), domain.Namespace("stub.test/u/r"))
	assert.ErrorIs(t, err, manifold.ErrUnexpectedPage)
}

func TestForgeHost_RawFile_TranslatesRawNotFound(t *testing.T) {
	host := adaptHost(stubForgeProvider{rawErr: provider.ErrRawNotFound}).(manifold.Forge)

	_, err := host.RawFile(context.Background(), domain.Namespace("stub.test/u/r"), "main", "ARROW.md")
	assert.ErrorIs(t, err, manifold.ErrRawNotFound)
}

func TestTranslateForgeErr_TableDriven(t *testing.T) {
	testCases := []struct {
		name string
		in   error
		want error
	}{
		{name: "raw not found", in: provider.ErrRawNotFound, want: manifold.ErrRawNotFound},
		{name: "unexpected page", in: provider.ErrUnexpectedPage, want: manifold.ErrUnexpectedPage},
		{name: "release not found", in: provider.ErrReleaseNotFound, want: manifold.ErrReleaseNotFound},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.ErrorIs(t, translateForgeErr(tc.in), tc.want)
		})
	}
}

func TestTranslateForgeErr_UnknownError_PassesThrough(t *testing.T) {
	original := errors.New("boom")
	assert.Same(t, original, translateForgeErr(original))
}
