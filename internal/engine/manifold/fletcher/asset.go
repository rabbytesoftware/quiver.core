package fletcher

import (
	"errors"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
)

var (
	// ErrNoAsset means no asset of the release is the one platform runs, or
	// several equally good ones remain.
	ErrNoAsset = errors.New("fletcher: no asset for the platform")

	// ErrNoDigest means the asset platform runs publishes no SHA-256 digest.
	ErrNoDigest = errors.New("fletcher: asset has no published digest")
)

// standInDigest lets the picker see an asset its host published no digest
// for, which it otherwise skips, so a missing digest can be told apart from a
// missing asset.
const standInDigest = "sha256:undigested"

// PickAsset chooses the one asset of a release that platform runs, using the
// same rules Fletcher drafts manifests with. An asset the host published no
// digest for is never returned: that is ErrNoDigest, not ErrNoAsset.
func PickAsset(
	repo string,
	assets []domain.ReleaseAsset,
	platform domain.OS,
) (domain.ReleaseAsset, error) {
	p := picker.New()
	if pick, ok := p.Pick(repo, assets, platform); ok {
		return pick.Asset, nil
	}

	standIn := make([]domain.ReleaseAsset, len(assets))
	for i, asset := range assets {
		if asset.Digest == "" {
			asset.Digest = standInDigest
		}
		standIn[i] = asset
	}
	if _, ok := p.Pick(repo, standIn, platform); ok {
		return domain.ReleaseAsset{}, ErrNoDigest
	}
	return domain.ReleaseAsset{}, ErrNoAsset
}
