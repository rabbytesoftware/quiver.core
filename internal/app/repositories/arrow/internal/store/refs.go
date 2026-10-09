package store

import (
	"context"
	"log/slog"
	"reflect"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const refsRefreshTimeout = 30 * time.Second

// Refs is ns's repository's tags and branches as a view needs them: the copy
// the vault saved, whatever its age, so a restart or an expired hour never
// makes a view wait on the host. A copy past the version-check TTL is read
// again behind the call; only a repository the vault never saved waits.
// Installs and adds read the host themselves.
func (r *storeService) Refs(
	ctx context.Context,
	ns domain.Namespace,
) (domain.RefSnapshot, error) {
	if r.vault == nil {
		return r.manifold.Snapshot(ctx, ns)
	}
	held, err := r.vault.GetRefs(ctx, ns)
	if err != nil {
		return r.saveRefs(ctx, ns, r.manifold.Snapshot)
	}
	if r.clock().Sub(held.CachedAt) > r.versionCheckTTL {
		r.refreshRefs(ctx, ns, held.Snapshot)
	}
	return held.Snapshot, nil
}

func (r *storeService) saveRefs(
	ctx context.Context,
	ns domain.Namespace,
	take snapshotFunc,
) (domain.RefSnapshot, error) {
	snap, err := take(ctx, ns)
	if err != nil {
		return domain.RefSnapshot{}, err
	}
	if err := r.vault.PutRefs(ctx, ns, snap); err != nil {
		slog.WarnContext(ctx, "store: save refs", "ns", ns, "err", err)
	}
	return snap, nil
}

func (r *storeService) refreshRefs(
	ctx context.Context,
	ns domain.Namespace,
	held domain.RefSnapshot,
) {
	bare := ns.BareNamespace()
	if _, busy := r.refreshing.LoadOrStore(bare, struct{}{}); busy {
		return
	}
	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), refsRefreshTimeout)
	r.running.Add(1)
	go func() {
		defer r.running.Done()
		defer r.refreshing.Delete(bare)
		defer cancel()
		fresh, err := r.saveRefs(refreshCtx, bare, r.manifold.FreshSnapshot)
		if err != nil {
			slog.WarnContext(refreshCtx, "store: refresh refs", "ns", bare, "err", err)
			return
		}
		if r.onRefreshed != nil && !reflect.DeepEqual(fresh, held) {
			r.onRefreshed(domain.Arrow{Namespace: bare})
		}
	}()
}
