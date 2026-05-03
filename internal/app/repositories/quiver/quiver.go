package quiver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/quiver/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/quiver/internal/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

type Quiver interface {
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
	) ([]domain.Quiver, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Quiver, error)
	OnQuiverAdded(fn func(
		ctx context.Context,
		q domain.Quiver),
	) error
	OnQuiverUpdated(fn func(
		ctx context.Context,
		q domain.Quiver,
	)) error
	OnQuiverRemoved(fn func(
		ctx context.Context,
		ns domain.Namespace,
	)) error
}

type quiverService struct {
	axQuiver asynx.Asynx[domain.Quiver]
	store    store.QuiverStore
	vault    vault.Vault
	manifold manifold.Manifold
}

// NewFromDBPath constructs a Quiver repo, creating an SQLite store at dbPath.
func NewFromDBPath(
	axQuiver asynx.Asynx[domain.Quiver],
	dbPath string,
	v vault.Vault,
	m manifold.Manifold,
) (Quiver, error) {
	s, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("quiver repository: open store: %w", err)
	}
	return New(axQuiver, s, v, m)
}

func New(
	axQuiver asynx.Asynx[domain.Quiver],
	s store.QuiverStore,
	v vault.Vault,
	m manifold.Manifold,
) (Quiver, error) {
	svc := &quiverService{
		axQuiver: axQuiver,
		store:    s,
		vault:    v,
		manifold: m,
	}

	if err := svc.registerProjections(); err != nil {
		return nil, fmt.Errorf("quiver repository: register projections: %w", err)
	}

	return svc, nil
}

func (s *quiverService) registerProjections() error {
	if _, err := s.axQuiver.Subscribe("quiver.added", func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
		_ = s.store.Save(ctx, evt.Aggregate)
	}); err != nil {
		return err
	}

	if _, err := s.axQuiver.Subscribe("quiver.updated", func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
		_ = s.store.Save(ctx, evt.Aggregate)
	}); err != nil {
		return err
	}

	if _, err := s.axQuiver.OnForget(func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
		if err := s.store.Delete(ctx, evt.Aggregate.Namespace); err != nil {
			slog.WarnContext(ctx, "forget: quiver store delete failed",
				"namespace", evt.Aggregate.Namespace, "err", err)
		}
	}); err != nil {
		return err
	}

	return nil
}

func (s *quiverService) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("add quiver: %w", apperrors.ErrInvalidNamespace)
	}

	_, _, err := s.resolveManifest(ctx, ns)
	if err != nil {
		return fmt.Errorf("add quiver: %w", apperrors.ErrFetchFailed)
	}

	if _, err := s.axQuiver.Send(ctx, commands.AddQuiver{
		Namespace: ns,
	}); err != nil {
		if errors.Is(err, asynxModels.ErrValidation) {
			return fmt.Errorf("add quiver: %w", apperrors.ErrAlreadyExists)
		}
		return fmt.Errorf("add quiver: %w", err)
	}

	return nil
}

func (s *quiverService) Update(
	ctx context.Context,
	ns domain.Namespace,
) error {
	exists, err := s.axQuiver.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("update quiver: %w", err)
	}
	if !exists {
		return fmt.Errorf("update quiver: %w", apperrors.ErrNotFound)
	}

	manifest, err := s.manifold.ResolveQuiver(ctx, ns)
	if err != nil {
		return fmt.Errorf("update quiver: %w", apperrors.ErrFetchFailed)
	}

	if _, err := s.vault.PutQuiver(ctx, ns, manifest); err != nil {
		return fmt.Errorf("update quiver: %w", err)
	}

	if _, err := s.axQuiver.Send(ctx, commands.UpdateQuiverManifest{
		Namespace: ns,
	}); err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return fmt.Errorf("update quiver: %w", apperrors.ErrNotFound)
		}
		return fmt.Errorf("update quiver: %w", err)
	}

	return nil
}

func (s *quiverService) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	exists, err := s.axQuiver.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("remove quiver: %w", err)
	}
	if !exists {
		return fmt.Errorf("remove quiver: %w", apperrors.ErrNotFound)
	}

	if err := s.axQuiver.Forget(ctx, ns.String()); err != nil {
		return fmt.Errorf("remove quiver: %w", err)
	}

	_ = s.vault.DeleteQuiver(ctx, ns)

	return nil
}

func (s *quiverService) List(ctx context.Context) ([]domain.Quiver, error) {
	return s.store.List(ctx)
}

func (s *quiverService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Quiver, error) {
	q, err := s.store.Get(ctx, ns)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperrors.ErrNotFound
	}

	return q, nil
}

func (s *quiverService) OnQuiverAdded(fn func(ctx context.Context, q domain.Quiver)) error {
	_, err := s.axQuiver.Subscribe("quiver.added", func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
		fn(ctx, evt.Aggregate)
	})
	return err
}

func (s *quiverService) OnQuiverUpdated(fn func(ctx context.Context, q domain.Quiver)) error {
	_, err := s.axQuiver.Subscribe("quiver.updated", func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
		fn(ctx, evt.Aggregate)
	})
	return err
}

func (s *quiverService) OnQuiverRemoved(fn func(ctx context.Context, ns domain.Namespace)) error {
	_, err := s.axQuiver.OnForget(func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
		fn(ctx, evt.Aggregate.Namespace)
	})
	return err
}

func (s *quiverService) resolveManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.QuiverManifest, string, error) {
	entry, homePath, err := s.vault.GetQuiver(ctx, ns)

	if err == nil {
		return entry.Manifest, homePath, nil
	}

	if errors.Is(err, vault.ErrStale) { //nolint:nestif
		manifest, manifoldErr := s.manifold.ResolveQuiver(ctx, ns)
		if manifoldErr != nil {
			return entry.Manifest, homePath, nil
		}

		newPath, putErr := s.vault.PutQuiver(ctx, ns, manifest)
		if putErr != nil {
			return nil, "", fmt.Errorf("resolveManifest: store refreshed manifest: %w", putErr)
		}

		return manifest, newPath, nil
	}

	if errors.Is(err, vault.ErrNotCached) { //nolint:nestif
		manifest, manifoldErr := s.manifold.ResolveQuiver(ctx, ns)
		if manifoldErr != nil {
			return nil, "", fmt.Errorf("resolveManifest: fetch from manifold: %w", manifoldErr)
		}

		newPath, putErr := s.vault.PutQuiver(ctx, ns, manifest)
		if putErr != nil {
			return nil, "", fmt.Errorf("resolveManifest: store manifest: %w", putErr)
		}

		return manifest, newPath, nil
	}

	return nil, "", fmt.Errorf("resolveManifest: vault lookup: %w", err)
}
