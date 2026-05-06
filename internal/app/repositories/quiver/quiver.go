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
	Follow(
		ctx context.Context,
		ns domain.Namespace,
		quiver *domain.Quiver,
		failedArrows []domain.Namespace,
	) error
	Unfollow(
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
	IsFollowed(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)
	OnQuiverFollowed(fn func(
		ctx context.Context,
		q domain.Quiver,
	)) error
	OnQuiverUnfollowed(fn func(
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
	if _, err := s.axQuiver.Subscribe("quiver.followed", func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
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

func (s *quiverService) Follow(
	ctx context.Context,
	ns domain.Namespace,
	quiver *domain.Quiver,
	failedArrows []domain.Namespace,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("follow quiver: %w", apperrors.ErrInvalidNamespace)
	}

	q := *quiver
	q.Namespace = ns
	q.FailedArrows = failedArrows

	if _, err := s.axQuiver.Send(ctx, commands.FollowQuiver{Quiver: q}); err != nil {
		if errors.Is(err, asynxModels.ErrValidation) {
			return fmt.Errorf("follow quiver: %w", apperrors.ErrAlreadyExists)
		}
		return fmt.Errorf("follow quiver: %w", err)
	}

	return nil
}

func (s *quiverService) Unfollow(
	ctx context.Context,
	ns domain.Namespace,
) error {
	exists, err := s.axQuiver.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("unfollow quiver: %w", err)
	}
	if !exists {
		return fmt.Errorf("unfollow quiver: %w", apperrors.ErrNotFound)
	}

	if err := s.axQuiver.Forget(ctx, ns.String()); err != nil {
		return fmt.Errorf("unfollow quiver: %w", err)
	}

	if err := s.vault.DeleteQuiver(ctx, ns); err != nil {
		slog.WarnContext(ctx, "unfollow: vault delete failed", "namespace", ns, "err", err)
	}

	return nil
}

func (s *quiverService) List(ctx context.Context) ([]domain.Quiver, error) {
	return s.store.List(ctx)
}

func (s *quiverService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Quiver, error) {
	// 1. asynx-first: followed quivers live in the event store
	q, err := s.axQuiver.Get(ctx, ns.String())
	if err == nil {
		return &q, nil
	}

	// 2. vault: unfollowed but cached quivers
	entry, _, vaultErr := s.vault.GetQuiver(ctx, ns)
	if vaultErr == nil {
		return entry.Quiver, nil
	}

	if errors.Is(vaultErr, vault.ErrStale) {
		return s.resolveStale(ctx, ns, entry.Quiver)
	}

	if errors.Is(vaultErr, vault.ErrNotCached) {
		return s.fetchAndCache(ctx, ns)
	}

	return nil, fmt.Errorf("get quiver: vault lookup: %w", vaultErr)
}

func (s *quiverService) resolveStale(
	ctx context.Context,
	ns domain.Namespace,
	stale *domain.Quiver,
) (*domain.Quiver, error) {
	quiver, err := s.manifold.ResolveQuiver(ctx, ns)
	if err != nil {
		return stale, nil
	}
	if _, putErr := s.vault.PutQuiver(ctx, ns, quiver); putErr != nil {
		return nil, fmt.Errorf("get quiver: store refreshed manifest: %w", putErr)
	}
	return quiver, nil
}

func (s *quiverService) fetchAndCache(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Quiver, error) {
	quiver, err := s.manifold.ResolveQuiver(ctx, ns)
	if err != nil {
		return nil, err
	}
	if _, putErr := s.vault.PutQuiver(ctx, ns, quiver); putErr != nil {
		return nil, fmt.Errorf("get quiver: store manifest: %w", putErr)
	}
	return quiver, nil
}

func (s *quiverService) IsFollowed(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error) {
	return s.axQuiver.Exists(ctx, ns.String())
}

func (s *quiverService) OnQuiverFollowed(fn func(ctx context.Context, q domain.Quiver)) error {
	_, err := s.axQuiver.Subscribe("quiver.followed", func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
		fn(ctx, evt.Aggregate)
	})
	return err
}

func (s *quiverService) OnQuiverUnfollowed(fn func(ctx context.Context, ns domain.Namespace)) error {
	_, err := s.axQuiver.OnForget(func(ctx context.Context, evt asynxModels.Event[domain.Quiver]) {
		fn(ctx, evt.Aggregate.Namespace)
	})
	return err
}
