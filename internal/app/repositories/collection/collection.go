package collection

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/collection/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/collection/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

type Collection interface {
	Follow(
		ctx context.Context,
		ns domain.Namespace,
		quiver *domain.Collection,
		failedArrows []domain.Namespace,
	) error
	Unfollow(
		ctx context.Context,
		ns domain.Namespace,
	) error
	List(
		ctx context.Context,
	) ([]domain.Collection, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Collection, error)
	IsFollowed(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)
	// Shutdown drains the collection aggregate, blocking until every in-flight
	// command has been persisted or ctx expires, then closes the collections
	// read model. Closing only after the drain keeps the last projection writes
	// off a closed handle; both phases run even when the first one fails.
	Shutdown(
		ctx context.Context,
	) error
	OnCollectionFollowed(fn func(
		ctx context.Context,
		q domain.Collection,
	)) error
	OnCollectionUnfollowed(fn func(
		ctx context.Context,
		ns domain.Namespace,
	)) error
}

type collectionService struct {
	axCollection asynx.Asynx[domain.Collection]
	store        store.QuiverStore
	vault        vault.Vault
	manifold     manifold.Manifold
}

func NewFromDBPath(
	axCollection asynx.Asynx[domain.Collection],
	dbPath string,
	v vault.Vault,
	m manifold.Manifold,
) (Collection, error) {
	s, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("collection repository: open store: %w", err)
	}
	return New(axCollection, s, v, m)
}

func New(
	axCollection asynx.Asynx[domain.Collection],
	s store.QuiverStore,
	v vault.Vault,
	m manifold.Manifold,
) (Collection, error) {
	svc := &collectionService{
		axCollection: axCollection,
		store:        s,
		vault:        v,
		manifold:     m,
	}

	if err := svc.registerProjections(); err != nil {
		return nil, fmt.Errorf("collection repository: register projections: %w", err)
	}

	return svc, nil
}

func (s *collectionService) registerProjections() error {
	if _, err := s.axCollection.Subscribe("collection.followed", func(ctx context.Context, evt asynxModels.Event[domain.Collection]) {
		_ = s.store.Save(ctx, evt.Aggregate)
	}); err != nil {
		return err
	}

	if _, err := s.axCollection.OnForget(func(ctx context.Context, evt asynxModels.Event[domain.Collection]) {
		if err := s.store.Delete(ctx, evt.Aggregate.Namespace); err != nil {
			slog.WarnContext(ctx, "forget: collection store delete failed",
				"namespace", evt.Aggregate.Namespace, "err", err)
		}
	}); err != nil {
		return err
	}

	return nil
}

func (s *collectionService) Follow(
	ctx context.Context,
	ns domain.Namespace,
	quiver *domain.Collection,
	failedArrows []domain.Namespace,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("follow quiver: %w", apperrors.ErrInvalidNamespace)
	}

	q := *quiver
	q.Namespace = ns
	q.FailedArrows = failedArrows

	if _, err := s.axCollection.Send(ctx, commands.FollowCollection{Collection: q}); err != nil {
		if errors.Is(err, asynxModels.ErrValidation) {
			return fmt.Errorf("follow quiver: %w", apperrors.ErrAlreadyExists)
		}
		return fmt.Errorf("follow quiver: %w", err)
	}

	return nil
}

func (s *collectionService) Unfollow(
	ctx context.Context,
	ns domain.Namespace,
) error {
	exists, err := s.axCollection.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("unfollow quiver: %w", err)
	}
	if !exists {
		return fmt.Errorf("unfollow quiver: %w", apperrors.ErrNotFound)
	}

	if err := s.axCollection.Forget(ctx, ns.String()); err != nil {
		return fmt.Errorf("unfollow quiver: %w", err)
	}

	if err := s.vault.DeleteCollection(ctx, ns); err != nil {
		slog.WarnContext(ctx, "unfollow: vault delete failed", "namespace", ns, "err", err)
	}

	return nil
}

func (s *collectionService) List(ctx context.Context) ([]domain.Collection, error) {
	return s.store.List(ctx)
}

func (s *collectionService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Collection, error) {
	// 1. asynx-first: followed quivers live in the event store
	q, err := s.axCollection.Get(ctx, ns.String())
	if err == nil {
		return &q, nil
	}

	// 2. vault: unfollowed but cached quivers
	entry, _, vaultErr := s.vault.GetCollection(ctx, ns)
	if vaultErr == nil {
		return entry.Collection, nil
	}

	if errors.Is(vaultErr, vault.ErrStale) {
		return s.resolveStale(ctx, ns, entry.Collection)
	}

	if errors.Is(vaultErr, vault.ErrNotCached) {
		return s.fetchAndCache(ctx, ns)
	}

	return nil, fmt.Errorf("get quiver: vault lookup: %w", vaultErr)
}

func (s *collectionService) resolveStale(
	ctx context.Context,
	ns domain.Namespace,
	stale *domain.Collection,
) (*domain.Collection, error) {
	quiver, err := s.manifold.ResolveCollection(ctx, ns)
	if err != nil {
		return stale, nil
	}
	if _, putErr := s.vault.PutCollection(ctx, ns, quiver); putErr != nil {
		return nil, fmt.Errorf("get quiver: store refreshed manifest: %w", putErr)
	}
	return quiver, nil
}

func (s *collectionService) fetchAndCache(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Collection, error) {
	quiver, err := s.manifold.ResolveCollection(ctx, ns)
	if err != nil {
		return nil, err
	}
	if _, putErr := s.vault.PutCollection(ctx, ns, quiver); putErr != nil {
		return nil, fmt.Errorf("get quiver: store manifest: %w", putErr)
	}
	return quiver, nil
}

func (s *collectionService) IsFollowed(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error) {
	return s.axCollection.Exists(ctx, ns.String())
}

func (s *collectionService) Shutdown(ctx context.Context) error {
	var errs []error

	if err := s.axCollection.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}

	if err := s.store.Close(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (s *collectionService) OnCollectionFollowed(fn func(ctx context.Context, q domain.Collection)) error {
	_, err := s.axCollection.Subscribe("collection.followed", func(ctx context.Context, evt asynxModels.Event[domain.Collection]) {
		fn(ctx, evt.Aggregate)
	})
	return err
}

func (s *collectionService) OnCollectionUnfollowed(fn func(ctx context.Context, ns domain.Namespace)) error {
	_, err := s.axCollection.OnForget(func(ctx context.Context, evt asynxModels.Event[domain.Collection]) {
		fn(ctx, evt.Aggregate.Namespace)
	})
	return err
}
