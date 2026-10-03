package assemblerinternal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
)

// ReleaseResolver answers release-bound variables from the release ns names.
// It takes every source a run needs at once so they all come from the same
// lookup: a rolling release replaced between two lookups would otherwise
// pair one build's URL with another's checksum.
type ReleaseResolver interface {
	Resolve(
		ctx context.Context,
		ns domain.Namespace,
		os domain.OS,
		sources []string,
	) (map[string]string, error)
}

// ReleaseAssetFn names the asset of ns's release that os runs.
type ReleaseAssetFn func(
	ctx context.Context,
	ns domain.Namespace,
	os domain.OS,
) (domain.ReleaseAsset, error)

type releaseResolver struct {
	asset ReleaseAssetFn
}

// NewReleaseResolver builds a ReleaseResolver over a release lookup.
func NewReleaseResolver(
	asset ReleaseAssetFn,
) ReleaseResolver {
	return &releaseResolver{asset: asset}
}

func (r *releaseResolver) Resolve(
	ctx context.Context,
	ns domain.Namespace,
	os domain.OS,
	sources []string,
) (map[string]string, error) {
	asset, err := r.asset(ctx, ns, os)
	if err != nil {
		return nil, classifyRelease(ctx, err)
	}

	resolved := make(map[string]string, len(sources))
	for _, source := range sources {
		switch source {
		case domain.VarSourceReleaseAsset:
			resolved[source] = asset.URL
		case domain.VarSourceReleaseChecksum:
			resolved[source] = strings.TrimPrefix(asset.Digest, "sha256:")
		default:
			return nil, fmt.Errorf("release source %q: %w", source, apperrors.ErrInvalidManifest)
		}
	}
	return resolved, nil
}

// classifyRelease types a failed lookup. An error that is not the release's
// own and not the host's (the caller giving up) passes through, so a
// cancelled run is not reported as an unreachable host.
func classifyRelease(
	ctx context.Context,
	err error,
) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return err
	}

	var limited *provider.RateLimitedError
	switch {
	case errors.As(err, &limited):
		return apperrors.NewReleaseError(apperrors.ReleaseRateLimited, err)
	case errors.Is(err, manifold.ErrNoRelease):
		return apperrors.NewReleaseError(apperrors.ReleaseNoRelease, err)
	case errors.Is(err, manifold.ErrNoAsset):
		return apperrors.NewReleaseError(apperrors.ReleaseNoAsset, err)
	case errors.Is(err, manifold.ErrUnsupportedPlatform):
		return apperrors.NewReleaseError(apperrors.ReleaseUnsupportedPlatform, err)
	case errors.Is(err, manifold.ErrUnverifiable):
		return apperrors.NewReleaseError(apperrors.ReleaseUnverifiable, err)
	default:
		return apperrors.NewReleaseError(apperrors.ReleaseOffline, err)
	}
}
