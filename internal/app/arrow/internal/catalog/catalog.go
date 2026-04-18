package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

// Catalog manages the arrow read model: add, update, remove, list, get, and
// dependency queries. Projection subscriptions are registered on construction.
type Catalog interface {
	Add(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Update(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Remove(
		ctx context.Context,
		ns domain.Namespace,
	) error
	List(
		ctx context.Context,
	) ([]domain.Arrow, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	HasDependents(
		ctx context.Context,
		ns domain.Namespace,
		excludeNs domain.Namespace,
	) (bool, error)
	AddWithManifest(
		ctx context.Context,
		ns domain.Namespace,
		manifest *domain.ArrowManifest,
	) error
	UpdateWithManifest(
		ctx context.Context,
		ns domain.Namespace,
		manifest *domain.ArrowManifest,
	) error
}

type catalogService struct {
	axArrow   asynx.Asynx[domain.Arrow]
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	store     store.ArrowCatalog
	vault     vault.Vault
	manifold  manifold.Manifold
}

func New(
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	cat store.ArrowCatalog,
	v vault.Vault,
	m manifold.Manifold,
) (Catalog, error) {
	c := &catalogService{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		store:     cat,
		vault:     v,
		manifold:  m,
	}

	if err := c.registerProjections(); err != nil {
		return nil, fmt.Errorf("catalog: register projections: %w", err)
	}

	return c, nil
}

func (c *catalogService) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("add arrow: %w", apperrors.ErrInvalidNamespace)
	}

	manifest, _, err := c.resolveManifest(ctx, ns)
	if err != nil {
		return fmt.Errorf("add arrow: %w", apperrors.ErrFetchFailed)
	}

	if _, err := c.axArrow.Send(ctx, arrowcmds.AddArrow{
		Namespace: ns,
		Manifest:  *manifest,
	}); err != nil {
		if errors.Is(err, asynxModels.ErrValidation) {
			return fmt.Errorf("add arrow: %w", apperrors.ErrAlreadyExists)
		}
		return fmt.Errorf("add arrow: %w", err)
	}

	return nil
}

func (c *catalogService) AddWithManifest(
	ctx context.Context,
	ns domain.Namespace,
	manifest *domain.ArrowManifest,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("add arrow with manifest: %w", apperrors.ErrInvalidNamespace)
	}

	if _, err := c.vault.PutArrow(ctx, ns, manifest); err != nil {
		return fmt.Errorf("add arrow with manifest: %w", err)
	}

	if _, err := c.axArrow.Send(ctx, arrowcmds.AddArrow{
		Namespace: ns,
		Manifest:  *manifest,
	}); err != nil {
		if errors.Is(err, asynxModels.ErrValidation) {
			return fmt.Errorf("add arrow with manifest: %w", apperrors.ErrAlreadyExists)
		}
		return fmt.Errorf("add arrow with manifest: %w", err)
	}

	return nil
}

func (c *catalogService) UpdateWithManifest(
	ctx context.Context,
	ns domain.Namespace,
	manifest *domain.ArrowManifest,
) error {
	if _, err := c.vault.PutArrow(ctx, ns, manifest); err != nil {
		return fmt.Errorf("update with manifest: %w", err)
	}

	if _, err := c.axArrow.Send(ctx, arrowcmds.UpdateArrowManifest{
		Namespace: ns,
		Manifest:  *manifest,
	}); err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) || errors.Is(err, asynxModels.ErrValidation) {
			return fmt.Errorf("update with manifest: %w", apperrors.ErrNotFound)
		}
		return fmt.Errorf("update with manifest: %w", err)
	}

	return nil
}

func (c *catalogService) Update(
	ctx context.Context,
	ns domain.Namespace,
) error {
	exists, err := c.axArrow.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("update arrow: %w", err)
	}
	if !exists {
		return fmt.Errorf("update arrow: %w", apperrors.ErrNotFound)
	}

	runtime, err := c.axRuntime.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return fmt.Errorf("update arrow: %w", err)
	}

	if runtime.Ref != "" && runtime.State != domain.ArrowStateReady {
		return fmt.Errorf("update arrow: %w", apperrors.ErrStateViolation)
	}

	manifest, err := c.manifold.ResolveArrow(ctx, ns)
	if err != nil {
		return fmt.Errorf("update arrow: %w", apperrors.ErrFetchFailed)
	}

	if _, err := c.vault.PutArrow(ctx, ns, manifest); err != nil {
		return fmt.Errorf("update arrow: %w", err)
	}

	if _, err := c.axArrow.Send(ctx, arrowcmds.UpdateArrowManifest{
		Namespace: ns,
		Manifest:  *manifest,
	}); err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return fmt.Errorf("update arrow: %w", apperrors.ErrNotFound)
		}
		return fmt.Errorf("update arrow: %w", err)
	}

	return nil
}

func (c *catalogService) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	exists, err := c.axArrow.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("remove arrow: %w", err)
	}
	if !exists {
		return fmt.Errorf("remove arrow: %w", apperrors.ErrNotFound)
	}

	runtime, err := c.axRuntime.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return fmt.Errorf("remove arrow: %w", err)
	}

	if runtime.Ref != "" {
		state := runtime.State
		if state != domain.ArrowStateAbsent && state != domain.ArrowStateRemoved && state != "" {
			return fmt.Errorf("remove arrow: %w", apperrors.ErrStateViolation)
		}
	}

	if err := c.axArrow.Forget(ctx, ns.String()); err != nil {
		return fmt.Errorf("remove arrow: %w", err)
	}

	// Best-effort: clean up runtime aggregate if it exists.
	if err := c.axRuntime.Forget(ctx, ns.String()); err != nil {
		slog.WarnContext(ctx, "remove arrow: runtime forget failed", "namespace", ns, "err", err)
	}

	_ = c.vault.DeleteArrow(ctx, ns) // best-effort

	return nil
}

func (c *catalogService) List(
	ctx context.Context,
) ([]domain.Arrow, error) {
	return c.store.List(ctx)
}

func (c *catalogService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	arrow, err := c.axArrow.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}

	return &arrow, nil
}

func (c *catalogService) HasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	arrows, err := c.store.List(ctx)
	if err != nil {
		return false, err
	}

	for _, arrow := range arrows {
		if arrow.Namespace == excludeNs || arrow.Namespace == ns {
			continue
		}

		rt, err := c.axRuntime.Get(ctx, arrow.Namespace.String())
		if err != nil || rt.State == domain.ArrowStateAbsent || rt.Ref == "" {
			continue
		}

		entry, _, err := c.vault.GetArrow(ctx, arrow.Namespace)
		if err != nil || entry.Manifest == nil {
			continue
		}

		for _, target := range entry.Manifest.Targets {
			for _, dep := range target.Tools {
				if dep.BareNamespace() == ns.BareNamespace() {
					return true, nil
				}
			}
			for _, dep := range target.Services {
				if dep.BareNamespace() == ns.BareNamespace() {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

func (c *catalogService) resolveManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.ArrowManifest, string, error) {
	entry, homePath, err := c.vault.GetArrow(ctx, ns)

	if err == nil {
		return entry.Manifest, homePath, nil
	}

	if errors.Is(err, vault.ErrStale) {
		manifest, manifoldErr := c.manifold.ResolveArrow(ctx, ns)
		if manifoldErr != nil {
			// Manifold unavailable — degrade gracefully to stale data.
			return entry.Manifest, homePath, nil
		}

		newPath, putErr := c.vault.PutArrow(ctx, ns, manifest, nil)
		if putErr != nil {
			return nil, "", fmt.Errorf("resolveManifest: store refreshed manifest: %w", putErr)
		}

		return manifest, newPath, nil
	}

	if errors.Is(err, vault.ErrNotCached) {
		manifest, manifoldErr := c.manifold.ResolveArrow(ctx, ns)
		if manifoldErr != nil {
			return nil, "", fmt.Errorf("resolveManifest: fetch from manifold: %w", manifoldErr)
		}

		newPath, putErr := c.vault.PutArrow(ctx, ns, manifest, nil)
		if putErr != nil {
			return nil, "", fmt.Errorf("resolveManifest: store manifest: %w", putErr)
		}

		return manifest, newPath, nil
	}

	return nil, "", fmt.Errorf("resolveManifest: vault lookup: %w", err)
}
