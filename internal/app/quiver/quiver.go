package quiver

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	quivercmds "github.com/rabbytesoftware/quiver/internal/app/quiver/commands"
	quiverstore "github.com/rabbytesoftware/quiver/internal/app/quiver/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

// QuiverService manages quiver lifecycle: registration, manifest updates, and removal.
type QuiverService interface {
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
	List(ctx context.Context) ([]QuiverListDTO, error)
	GetDetail(
		ctx context.Context,
		ns domain.Namespace,
	) (*QuiverDetailDTO, error)
}

type quiverService struct {
	asynxQuiver asynx.Asynx[domain.Quiver]
	catalog     quiverstore.QuiverCatalog
	engines     engine.Container
}

func (svc *quiverService) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("add quiver: %w", ErrInvalidNamespace)
	}

	manifest, _, err := svc.resolveManifest(ctx, ns)
	if err != nil {
		return fmt.Errorf("add quiver: %w", ErrFetchFailed)
	}

	if _, err := svc.asynxQuiver.Send(ctx, quivercmds.AddQuiver{
		Namespace: ns,
		Manifest:  *manifest,
	}); err != nil {
		return fmt.Errorf("add quiver: %w", err)
	}

	return nil
}

func (svc *quiverService) Update(
	ctx context.Context,
	ns domain.Namespace,
) error {
	current, err := svc.asynxQuiver.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return fmt.Errorf("update quiver: %w", ErrNotFound)
		}
		return fmt.Errorf("update quiver: %w", err)
	}

	if current.Removed {
		return fmt.Errorf("update quiver: %w", ErrAlreadyRemoved)
	}

	manifest, err := svc.engines.Manifold.ResolveQuiver(ctx, ns)
	if err != nil {
		return fmt.Errorf("update quiver: %w", ErrFetchFailed)
	}

	if _, err := svc.engines.Vault.PutQuiver(ctx, ns, manifest); err != nil {
		return fmt.Errorf("update quiver: %w", err)
	}

	if _, err := svc.asynxQuiver.Send(ctx, quivercmds.UpdateQuiverManifest{
		Namespace: ns,
		Manifest:  *manifest,
	}); err != nil {
		return fmt.Errorf("update quiver: %w", err)
	}

	return nil
}

func (svc *quiverService) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	current, err := svc.asynxQuiver.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return fmt.Errorf("remove quiver: %w", ErrNotFound)
		}
		return fmt.Errorf("remove quiver: %w", err)
	}

	if current.Removed {
		return fmt.Errorf("remove quiver: %w", ErrAlreadyRemoved)
	}

	if _, err := svc.asynxQuiver.Send(ctx, quivercmds.RemoveQuiver{Namespace: ns}); err != nil {
		return fmt.Errorf("remove quiver: %w", err)
	}

	_ = svc.engines.Vault.DeleteQuiver(ctx, ns)

	return nil
}

func (svc *quiverService) List(ctx context.Context) ([]QuiverListDTO, error) {
	quivers, err := svc.catalog.List(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]QuiverListDTO, 0, len(quivers))
	for _, quiver := range quivers {
		if quiver.Removed {
			continue
		}

		result = append(result, QuiverListDTO{
			Namespace:   quiver.Namespace,
			Name:        quiver.Manifest.Name,
			Description: quiver.Manifest.Description,
			Tags:        quiver.Manifest.Tags,
			Removed:     quiver.Removed,
		})
	}

	return result, nil
}

func (svc *quiverService) GetDetail(
	ctx context.Context,
	ns domain.Namespace,
) (*QuiverDetailDTO, error) {
	quiver, err := svc.catalog.Get(ctx, ns)
	if err != nil {
		return nil, err
	}
	if quiver == nil {
		return nil, fmt.Errorf("get detail: %w", ErrNotFound)
	}

	return &QuiverDetailDTO{
		Namespace: quiver.Namespace,
		Manifest:  quiver.Manifest,
		Removed:   quiver.Removed,
	}, nil
}

func (svc *quiverService) resolveManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.QuiverManifest, string, error) {
	entry, homePath, err := svc.engines.Vault.GetQuiver(ctx, ns)

	if err == nil {
		return entry.Manifest, homePath, nil
	}

	if errors.Is(err, vault.ErrStale) {
		manifest, manifoldErr := svc.engines.Manifold.ResolveQuiver(ctx, ns)
		if manifoldErr != nil {
			return entry.Manifest, homePath, nil
		}

		newPath, putErr := svc.engines.Vault.PutQuiver(ctx, ns, manifest)
		if putErr != nil {
			return nil, "", fmt.Errorf("resolveManifest: store refreshed manifest: %w", putErr)
		}

		return manifest, newPath, nil
	}

	if errors.Is(err, vault.ErrNotCached) {
		manifest, manifoldErr := svc.engines.Manifold.ResolveQuiver(ctx, ns)
		if manifoldErr != nil {
			return nil, "", fmt.Errorf("resolveManifest: fetch from manifold: %w", manifoldErr)
		}

		newPath, putErr := svc.engines.Vault.PutQuiver(ctx, ns, manifest)
		if putErr != nil {
			return nil, "", fmt.Errorf("resolveManifest: store manifest: %w", putErr)
		}

		return manifest, newPath, nil
	}

	return nil, "", fmt.Errorf("resolveManifest: vault lookup: %w", err)
}
