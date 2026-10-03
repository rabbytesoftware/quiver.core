package manifold

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type assetHost struct {
	stubHost
	assets []domain.ReleaseAsset
	err    error
	tags   []string
}

func (h *assetHost) ReleaseAssets(
	_ context.Context,
	_ domain.Namespace,
	tag string,
) ([]domain.ReleaseAsset, error) {
	h.tags = append(h.tags, tag)
	return h.assets, h.err
}

func releaseAssets(withDigests bool) []domain.ReleaseAsset {
	names := []string{
		"quiver-linux-amd64",
		"quiver-linux-arm64",
		"quiver-darwin-amd64",
		"quiver-darwin-arm64",
		"quiver-windows-amd64.exe",
	}
	assets := make([]domain.ReleaseAsset, len(names))
	for i, name := range names {
		assets[i] = domain.ReleaseAsset{Name: name, URL: "https://example.test/dl/" + name}
		if withDigests {
			assets[i].Digest = "sha256:" + name
		}
	}
	return append(assets, domain.ReleaseAsset{Name: "checksums.txt", URL: "https://example.test/dl/checksums.txt"})
}

func releaseManifold(h *assetHost) Manifold {
	return New(0, func(_ domain.Namespace) (Host, bool) { return h, true }, 0)
}

func TestManifold_ResolveReleaseAsset_PicksThePlatformsAsset(t *testing.T) {
	testCases := []struct {
		os   domain.OS
		want string
	}{
		{domain.OSLinuxAMD64, "quiver-linux-amd64"},
		{domain.OSDarwinARM64, "quiver-darwin-arm64"},
		{domain.OSWindowsAMD64, "quiver-windows-amd64.exe"},
	}

	for _, tc := range testCases {
		t.Run(tc.os.String(), func(t *testing.T) {
			host := &assetHost{assets: releaseAssets(true)}

			asset, err := releaseManifold(host).ResolveReleaseAsset(
				context.Background(),
				domain.Namespace("github.com/rabbytesoftware/quiver.core@nightly-latest"),
				tc.os,
			)

			require.NoError(t, err)
			assert.Equal(t, tc.want, asset.Name)
			assert.Equal(t, []string{"nightly-latest"}, host.tags, "the release is the namespace's ref")
		})
	}
}

func TestManifold_ResolveReleaseAsset_Failures(t *testing.T) {
	ns := domain.Namespace("github.com/rabbytesoftware/quiver.core@nightly-latest")
	hostErr := errors.New("connection refused")

	testCases := []struct {
		name    string
		host    *assetHost
		lookup  HostLookup
		ns      domain.Namespace
		os      domain.OS
		wantErr error
	}{
		{name: "no ref", host: &assetHost{}, ns: domain.Namespace("github.com/rabbytesoftware/quiver.core"), os: domain.OSLinuxAMD64, wantErr: ErrNoRelease},
		{name: "no host", host: &assetHost{}, lookup: func(_ domain.Namespace) (Host, bool) { return nil, false }, ns: ns, os: domain.OSLinuxAMD64, wantErr: ErrNoRelease},
		{name: "empty release", host: &assetHost{}, ns: ns, os: domain.OSLinuxAMD64, wantErr: ErrNoRelease},
		{name: "host failure", host: &assetHost{err: hostErr}, ns: ns, os: domain.OSLinuxAMD64, wantErr: hostErr},
		{name: "platform without asset", host: &assetHost{assets: releaseAssets(true)[:1]}, ns: ns, os: domain.OSDarwinARM64, wantErr: ErrUnsupportedPlatform},
		{name: "no digest", host: &assetHost{assets: releaseAssets(false)}, ns: ns, os: domain.OSLinuxAMD64, wantErr: ErrUnverifiable},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := tc.lookup
			if lookup == nil {
				lookup = func(_ domain.Namespace) (Host, bool) { return tc.host, true }
			}

			_, err := New(0, lookup, 0).ResolveReleaseAsset(context.Background(), tc.ns, tc.os)

			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestManifold_ResolveReleaseAsset_AssetWithoutURL(t *testing.T) {
	assets := releaseAssets(true)
	for i := range assets {
		assets[i].URL = ""
	}

	_, err := releaseManifold(&assetHost{assets: assets}).ResolveReleaseAsset(
		context.Background(),
		domain.Namespace("github.com/rabbytesoftware/quiver.core@nightly-latest"),
		domain.OSLinuxAMD64,
	)

	require.ErrorIs(t, err, ErrNoAsset)
}
