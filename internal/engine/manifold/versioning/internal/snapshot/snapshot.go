package snapshot

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
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

// refreshTimeout bounds a refresh nobody is waiting on.
const refreshTimeout = 30 * time.Second

// Snapshots reads a repository's refs, cached for a TTL. Snapshot answers from
// what it holds, in memory or on disk, whatever its age, and re-reads an
// expired one behind the call; only a repository it has never seen waits on
// the host. FreshSnapshot always waits.
type Snapshots interface {
	Snapshot(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.RefSnapshot, error)

	// Wait blocks until every background re-read has finished.
	Wait()

	// Persist keeps snapshots in dir, so a restart starts with them.
	Persist(dir string)

	// OnRefreshed reports a repository whose refs a background re-read found
	// changed.
	OnRefreshed(fn func(ns domain.Namespace))

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

	disk        disk
	running     sync.WaitGroup
	inflight    sync.Map
	onRefreshed atomic.Pointer[func(domain.Namespace)]
}

// New builds Snapshots that read refs and reuse a result for ttl by clock.
func New(
	refs RefLister,
	clock func() time.Time,
	ttl time.Duration,
) Snapshots {
	return &snapshots{refs: refs, clock: clock, ttl: ttl}
}

func (s *snapshots) Wait() {
	s.running.Wait()
}

func (s *snapshots) Persist(
	dir string,
) {
	s.disk = disk{dir: dir}
}

func (s *snapshots) OnRefreshed(
	fn func(ns domain.Namespace),
) {
	s.onRefreshed.Store(&fn)
}

func (s *snapshots) Snapshot(
	ctx context.Context,
	ns domain.Namespace,
) (domain.RefSnapshot, error) {
	bare := ns.BareNamespace()
	held, ok := s.held(ctx, bare)
	if !ok {
		return s.liveSnapshot(ctx, bare)
	}
	if s.clock().Sub(held.cachedAt) > s.ttl {
		s.refreshBehind(ctx, bare, held)
	}
	return held.snap, nil
}

// held is what is known of bare without asking the host: memory first, then
// the disk, which it brings back into memory.
func (s *snapshots) held(
	ctx context.Context,
	bare domain.Namespace,
) (entry, bool) {
	if v, ok := s.cache.Load(bare); ok {
		cached, _ := v.(entry)
		return cached, true
	}
	loaded, ok := s.disk.load(ctx, bare)
	if ok {
		s.cache.Store(bare, loaded)
	}
	return loaded, ok
}

func (s *snapshots) refreshBehind(
	ctx context.Context,
	bare domain.Namespace,
	held entry,
) {
	if _, busy := s.inflight.LoadOrStore(bare, struct{}{}); busy {
		return
	}
	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), refreshTimeout)
	s.running.Add(1)
	go func() {
		defer s.running.Done()
		defer s.inflight.Delete(bare)
		defer cancel()
		fresh, err := s.liveSnapshot(refreshCtx, bare)
		if err != nil {
			slog.WarnContext(refreshCtx, "snapshot: refresh refs", "ns", bare, "err", err)
			return
		}
		if fn := s.onRefreshed.Load(); fn != nil && !reflect.DeepEqual(fresh, held.snap) {
			(*fn)(bare)
		}
	}()
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

	fresh := entry{snap: snap, cachedAt: s.clock()}
	s.cache.Store(bare, fresh)
	s.disk.save(ctx, bare, fresh)
	return snap, nil
}
