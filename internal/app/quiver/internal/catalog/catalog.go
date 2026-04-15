package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/app/quiver/internal/catalog/store"
	quivercmds "github.com/rabbytesoftware/quiver/internal/app/quiver/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

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
	List(ctx context.Context) ([]domain.Quiver, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Quiver, error)
}

type catalogService struct {
	axQuiver asynx.Asynx[domain.Quiver]
	store    store.QuiverCatalog
	vault    vault.Vault
	manifold manifold.Manifold
}

func New(
	axQuiver asynx.Asynx[domain.Quiver],
	cat store.QuiverCatalog,
	v vault.Vault,
	m manifold.Manifold,
) (Catalog, error) {
	c := &catalogService{
		axQuiver: axQuiver,
		store:    cat,
		vault:    v,
		manifold: m,
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
		return fmt.Errorf("add quiver: %w", apperrors.ErrInvalidNamespace)
	}

	manifest, _, err := c.resolveManifest(ctx, ns)
	if err != nil {
		return fmt.Errorf("add quiver: %w", apperrors.ErrFetchFailed)
	}

	if _, err := c.axQuiver.Send(ctx, quivercmds.AddQuiver{
		Namespace: ns,
		Manifest:  *manifest,
	}); err != nil {
		if errors.Is(err, asynxModels.ErrValidation) {
			return fmt.Errorf("add quiver: %w", apperrors.ErrAlreadyExists)
		}
		return fmt.Errorf("add quiver: %w", err)
	}

	return nil
}

func (c *catalogService) Update(
	ctx context.Context,
	ns domain.Namespace,
) error {
	_, err := c.axQuiver.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return fmt.Errorf("update quiver: %w", apperrors.ErrNotFound)
		}
		return fmt.Errorf("update quiver: %w", err)
	}

	manifest, err := c.manifold.ResolveQuiver(ctx, ns)
	if err != nil {
		return fmt.Errorf("update quiver: %w", apperrors.ErrFetchFailed)
	}

	if _, err := c.vault.PutQuiver(ctx, ns, manifest); err != nil {
		return fmt.Errorf("update quiver: %w", err)
	}

	if _, err := c.axQuiver.Send(ctx, quivercmds.UpdateQuiverManifest{
		Namespace: ns,
		Manifest:  *manifest,
	}); err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return fmt.Errorf("update quiver: %w", apperrors.ErrNotFound)
		}
		return fmt.Errorf("update quiver: %w", err)
	}

	return nil
}

func (c *catalogService) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	_, err := c.axQuiver.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return fmt.Errorf("remove quiver: %w", apperrors.ErrNotFound)
		}
		return fmt.Errorf("remove quiver: %w", err)
	}

	if err := c.axQuiver.Forget(ctx, ns.String()); err != nil {
		return fmt.Errorf("remove quiver: %w", err)
	}

	_ = c.vault.DeleteQuiver(ctx, ns)

	return nil
}

func (c *catalogService) List(ctx context.Context) ([]domain.Quiver, error) {
	return c.store.List(ctx)
}

func (c *catalogService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Quiver, error) {
	quiver, err := c.store.Get(ctx, ns)
	if err != nil {
		return nil, err
	}
	if quiver == nil {
		return nil, apperrors.ErrNotFound
	}

	return quiver, nil
}

func (c *catalogService) resolveManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.QuiverManifest, string, error) {
	entry, homePath, err := c.vault.GetQuiver(ctx, ns)

	if err == nil {
		return entry.Manifest, homePath, nil
	}

	if errors.Is(err, vault.ErrStale) {
		manifest, manifoldErr := c.manifold.ResolveQuiver(ctx, ns)
		if manifoldErr != nil {
			return entry.Manifest, homePath, nil
		}

		newPath, putErr := c.vault.PutQuiver(ctx, ns, manifest)
		if putErr != nil {
			return nil, "", fmt.Errorf("resolveManifest: store refreshed manifest: %w", putErr)
		}

		return manifest, newPath, nil
	}

	if errors.Is(err, vault.ErrNotCached) {
		manifest, manifoldErr := c.manifold.ResolveQuiver(ctx, ns)
		if manifoldErr != nil {
			return nil, "", fmt.Errorf("resolveManifest: fetch from manifold: %w", manifoldErr)
		}

		newPath, putErr := c.vault.PutQuiver(ctx, ns, manifest)
		if putErr != nil {
			return nil, "", fmt.Errorf("resolveManifest: store manifest: %w", putErr)
		}

		return manifest, newPath, nil
	}

	return nil, "", fmt.Errorf("resolveManifest: vault lookup: %w", err)
}
