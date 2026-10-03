package manifold

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
)

// The reasons a release cannot name an asset to run. A host that could not be
// asked is not among them: that error is returned as the host gave it, so a
// caller can tell a rate limit from an outage.
var (
	// ErrNoRelease reports a namespace without a ref, without a known host, or
	// whose ref has no published release.
	ErrNoRelease = errors.New("manifold: no release")

	// ErrNoAsset reports that the asset chosen carries no download address.
	ErrNoAsset = errors.New("manifold: release asset has no download url")

	// ErrUnsupportedPlatform reports a release none of whose assets runs on the
	// platform asked for.
	ErrUnsupportedPlatform = errors.New("manifold: no release asset for this platform")

	// ErrUnverifiable reports that the asset for the platform publishes no
	// SHA-256 digest, so a download of it could not be checked.
	ErrUnverifiable = errors.New("manifold: release asset has no published digest")
)

func (m *manifold) ResolveReleaseAsset(
	ctx context.Context,
	ns domain.Namespace,
	os domain.OS,
) (domain.ReleaseAsset, error) {
	tag := ns.Ref()
	if tag == "" {
		return domain.ReleaseAsset{}, fmt.Errorf("manifold: release of %s: %w: the namespace names no ref", ns, ErrNoRelease)
	}
	host, ok := m.hosts(ns)
	if !ok {
		return domain.ReleaseAsset{}, fmt.Errorf("manifold: release of %s: %w: no host serves it", ns, ErrNoRelease)
	}

	assets, err := host.ReleaseAssets(ctx, ns, tag)
	if err != nil {
		return domain.ReleaseAsset{}, fmt.Errorf("manifold: release assets of %s: %w", ns, err)
	}
	if len(assets) == 0 {
		return domain.ReleaseAsset{}, fmt.Errorf("manifold: release %s of %s: %w: it publishes no assets", tag, ns, ErrNoRelease)
	}

	asset, err := fletcher.PickAsset(repoName(ns), assets, os)
	if errors.Is(err, fletcher.ErrNoDigest) {
		return domain.ReleaseAsset{}, fmt.Errorf("manifold: release %s of %s on %s: %w", tag, ns, os, ErrUnverifiable)
	}
	if err != nil {
		return domain.ReleaseAsset{}, fmt.Errorf("manifold: release %s of %s on %s: %w", tag, ns, os, ErrUnsupportedPlatform)
	}
	if asset.URL == "" {
		return domain.ReleaseAsset{}, fmt.Errorf("manifold: asset %s of %s: %w", asset.Name, ns, ErrNoAsset)
	}
	return asset, nil
}

func repoName(
	ns domain.Namespace,
) string {
	segments := append(strings.SplitN(ns.BareNamespace().String(), domain.NamespaceSeparator, 4), "", "")
	return segments[2]
}
