package gather

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
)

func isRolling(
	tag string,
) bool {
	if tag == "" {
		return false
	}
	return !resolvers.HasVersionCore(tag)
}

func withStandInDigests(
	assets []domain.ReleaseAsset,
) []domain.ReleaseAsset {
	out := make([]domain.ReleaseAsset, len(assets))
	for i, a := range assets {
		if a.Digest == "" {
			sum := sha256.Sum256([]byte(a.URL + "\x00" + a.Name))
			a.Digest = "sha256:" + hex.EncodeToString(sum[:])
		}
		out[i] = a
	}
	return out
}
