package snapshot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// RefLister reads every ref of a repository in one round trip.
type RefLister interface {
	Refs(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.RefSnapshot, error)
}

// Snapshots reads a repository's refs, cached for a TTL.
type Snapshots interface {
	Snapshot(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.RefSnapshot, error)

	FreshSnapshot(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.RefSnapshot, error)
}

type snapshots struct {
	refs  RefLister
	clock func() time.Time
	ttl   time.Duration

	// cache holds one entry per bare domain.Namespace queried through
	// Snapshot (ListChannels included). It is a sync.Map, not a
	// mutex-guarded map, since entries are independent of each other and
	// never iterated as a whole — a process-lifetime, unbounded cache (no
	// eviction beyond TTL-on-read); a long-running daemon queried for many
	// distinct namespaces will grow it accordingly, which is accepted for
	// this simple, restart-safe-to-lose optimization rather than an LRU or
	// similar bound.
	cache sync.Map
}

// New builds Snapshots that read refs and reuse a result for ttl by clock.
func New(
	refs RefLister,
	clock func() time.Time,
	ttl time.Duration,
) Snapshots {
	return &snapshots{refs: refs, clock: clock, ttl: ttl}
}

func (s *snapshots) Snapshot(
	ctx context.Context,
	ns domain.Namespace,
) (domain.RefSnapshot, error) {
	bare := ns.BareNamespace()
	if cached, ok := s.cachedSnapshot(bare); ok {
		return cached, nil
	}
	return s.liveSnapshot(ctx, bare)
}

func (s *snapshots) FreshSnapshot(
	ctx context.Context,
	ns domain.Namespace,
) (domain.RefSnapshot, error) {
	return s.liveSnapshot(ctx, ns.BareNamespace())
}

func (s *snapshots) liveSnapshot(
	ctx context.Context,
	bare domain.Namespace,
) (domain.RefSnapshot, error) {
	snap, err := s.refs.Refs(ctx, bare)
	if err != nil {
		return domain.RefSnapshot{}, fmt.Errorf("manifold: snapshot %s: %w", bare, err)
	}

	s.cache.Store(bare, entry{snap: snap, cachedAt: s.clock()})
	return snap, nil
}

// cachedSnapshot returns the still-fresh snapshot of bare, if one exists. The
// maps inside it are shared with the cache, so callers treat it as read-only.
func (s *snapshots) cachedSnapshot(
	bare domain.Namespace,
) (domain.RefSnapshot, bool) {
	v, ok := s.cache.Load(bare)
	if !ok {
		return domain.RefSnapshot{}, false
	}
	cached, _ := v.(entry)
	if s.clock().Sub(cached.cachedAt) > s.ttl {
		return domain.RefSnapshot{}, false
	}
	return cached.snap, true
}
