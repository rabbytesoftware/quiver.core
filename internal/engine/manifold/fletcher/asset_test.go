package fletcher_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
)

func TestPickAsset(t *testing.T) {
	asset := func(name, digest string) domain.ReleaseAsset {
		return domain.ReleaseAsset{Name: name, URL: "https://example.test/" + name, Digest: digest}
	}

	testCases := []struct {
		name    string
		assets  []domain.ReleaseAsset
		os      domain.OS
		want    string
		wantErr error
	}{
		{
			name:   "digested asset for the platform",
			assets: []domain.ReleaseAsset{asset("tool-linux-amd64", "sha256:aa"), asset("tool-darwin-arm64", "sha256:bb")},
			os:     domain.OSDarwinARM64,
			want:   "tool-darwin-arm64",
		},
		{
			name:    "platform absent",
			assets:  []domain.ReleaseAsset{asset("tool-linux-amd64", "sha256:aa")},
			os:      domain.OSDarwinARM64,
			wantErr: fletcher.ErrNoAsset,
		},
		{
			name:    "asset for the platform has no digest",
			assets:  []domain.ReleaseAsset{asset("tool-linux-amd64", "")},
			os:      domain.OSLinuxAMD64,
			wantErr: fletcher.ErrNoDigest,
		},
		{
			name:    "no assets",
			os:      domain.OSLinuxAMD64,
			wantErr: fletcher.ErrNoAsset,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := fletcher.PickAsset("tool", tc.assets, tc.os)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Name)
		})
	}
}
