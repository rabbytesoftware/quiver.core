package collection

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/app/bind"
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
	// read model. Both phases run even when the first one fails.
	//
	// Closing after the drain is what keeps the last projection writes off a
	// closed handle — on a drain that completed. A drain that ran out of ctx has
	// stopped waiting, not stopped writing, and those writes do hit the closed
	// handle; they replay from the event store on the next boot.
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

	// asynx runs one goroutine per subscriber, so a second subscription on the
	// same topic would race the read-model write. Callbacks are held here and
	// invoked by the single projection instead, after the write has landed.
	synced bool

	callbacksMu   sync.RWMutex
	followedFns   []func(ctx context.Context, q domain.Collection)
	unfollowedFns []func(ctx context.Context, ns domain.Namespace)
}

func NewFromDBPath(
	axCollection asynx.Asynx[domain.Collection],
	dbPath string,
	v vault.Vault,
	m manifold.Manifold,
) (Collection, error) {
	s, err := openStore(axCollection, dbPath)
	if err != nil {
		return nil, fmt.Errorf("collection repository: open store: %w", err)
	}
	return New(axCollection, s, v, m)
}

func openStore(
	ax asynx.Asynx[domain.Collection],
	dbPath string,
) (store.QuiverStore, error) {
	host := bind.Host(ax)
	if host == nil {
		return store.New(dbPath)
	}
	return store.NewHosted(host, dbPath)
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
	err := s.registerSyncProjection()
	if err != nil {
		return err
	}

	err = bind.OnEvent(s.axCollection, "collection.followed", s.projectFollowed)
	if err != nil {
		return err
	}

	_, err = s.axCollection.OnForget(s.projectForgotten)
	return err
}

func (s *collectionService) registerSyncProjection() error {
	saver, ok := s.store.(store.TxSaver)
	if !ok {
		return nil
	}
	save := func(ctx context.Context, tx asynxModels.Tx, evt asynxModels.Event[domain.Collection]) error {
		return saver.SaveTx(ctx, tx, evt.Aggregate)
	}
	synced, err := bind.Projection(s.axCollection, "collection.followed", save)
	s.synced = synced
	return err
}

func (s *collectionService) projectFollowed(
	ctx context.Context,
	evt asynxModels.Event[domain.Collection],
) error {
	err := s.saveUnlessSynced(ctx, evt.Aggregate)
	if err != nil {
		return err
	}

	for _, fn := range s.followedCallbacks() {
		fn(ctx, evt.Aggregate)
	}
	return nil
}

func (s *collectionService) saveUnlessSynced(
	ctx context.Context,
	coll domain.Collection,
) error {
	if s.synced {
		return nil
	}
	err := s.store.Save(ctx, coll)
	if err == nil {
		return nil
	}
	slog.ErrorContext(ctx, "follow: collection store save failed",
		"namespace", coll.Namespace, "err", err)
	return fmt.Errorf("follow: save collection %s: %w", coll.Namespace, err)
}

// projectForgotten mirrors projectFollowed for removal: a delete that failed
// leaves the row readable, so announcing the removal would contradict the state
// the read model still serves.
func (s *collectionService) projectForgotten(
	ctx context.Context,
	evt asynxModels.Event[domain.Collection],
) {
	if err := s.store.Delete(ctx, evt.Aggregate.Namespace); err != nil {
		slog.WarnContext(ctx, "forget: collection store delete failed",
			"namespace", evt.Aggregate.Namespace, "err", err)
		return
	}

	for _, fn := range s.unfollowedCallbacks() {
		fn(ctx, evt.Aggregate.Namespace)
	}
}

// followedCallbacks snapshots the registered callbacks so the projection never
// holds the lock while running them.
func (s *collectionService) followedCallbacks() []func(ctx context.Context, q domain.Collection) {
	s.callbacksMu.RLock()
	defer s.callbacksMu.RUnlock()
	return slices.Clone(s.followedFns)
}

func (s *collectionService) unfollowedCallbacks() []func(ctx context.Context, ns domain.Namespace) {
	s.callbacksMu.RLock()
	defer s.callbacksMu.RUnlock()
	return slices.Clone(s.unfollowedFns)
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
		return fmt.Errorf("unfollow quiver: %w", apperrors.FromForget(err))
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
		errs = append(errs, fmt.Errorf("collection repository: drain aggregate: %w", err))
	}

	if err := s.store.Close(); err != nil {
		errs = append(errs, fmt.Errorf("collection repository: close store: %w", err))
	}

	return errors.Join(errs...)
}

// OnCollectionFollowed registers fn on the projection that owns the read-model
// write. Callbacks run in registration order once the write has landed.
func (s *collectionService) OnCollectionFollowed(fn func(ctx context.Context, q domain.Collection)) error {
	s.callbacksMu.Lock()
	defer s.callbacksMu.Unlock()
	s.followedFns = append(s.followedFns, fn)
	return nil
}

// OnCollectionUnfollowed registers fn on the forget projection. Callbacks run in
// registration order once the read-model row is gone.
func (s *collectionService) OnCollectionUnfollowed(fn func(ctx context.Context, ns domain.Namespace)) error {
	s.callbacksMu.Lock()
	defer s.callbacksMu.Unlock()
	s.unfollowedFns = append(s.unfollowedFns, fn)
	return nil
}
