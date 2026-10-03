package runtime

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type GetArrowFn func(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error)

type MarkInstalledFn func(
	ctx context.Context,
	ns domain.Namespace,
	at time.Time,
) error

type MarkUninstalledFn func(
	ctx context.Context,
	ns domain.Namespace,
) error

type MarkLastUsedFn func(
	ctx context.Context,
	ns domain.Namespace,
	at time.Time,
) error

type HasDependentsFn func(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error)

type ListArrowsFn func(ctx context.Context) ([]models.ArrowView, error)

// ListRuntimeAggregatesFn returns the namespaces of every runtime aggregate
// that currently has events in the runtime event store.
type ListRuntimeAggregatesFn func(ctx context.Context) ([]domain.Namespace, error)

// RefreshManifestFn resolves ns's manifest again from its host, at the
// release the method is running — the installed one for an install, the
// update's target for an update — and stages it on the row.
type RefreshManifestFn func(
	ctx context.Context,
	ns domain.Namespace,
	method string,
) error

// ReleaseAssetFn names the asset of the release ns's ref publishes that os
// runs, for the variables a manifest binds to its release.
type ReleaseAssetFn func(
	ctx context.Context,
	ns domain.Namespace,
	os domain.OS,
) (domain.ReleaseAsset, error)

type options struct {
	releaseAsset ReleaseAssetFn
}

// Option configures New.
type Option func(*options)

// WithReleaseAsset lets variables that declare a release source be filled
// from the release a run is built from.
func WithReleaseAsset(
	fn ReleaseAssetFn,
) Option {
	return func(o *options) { o.releaseAsset = fn }
}
