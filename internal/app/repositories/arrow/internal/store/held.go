package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

const recheckTimeout = 30 * time.Second

func (r *storeService) resolveRefless(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	if held, ok := r.heldPreview(ctx, ns); ok {
		return held, nil
	}
	identity, arrow, err := r.ResolveInstall(ctx, ns, Preview())
	if err != nil {
		return nil, fmt.Errorf("reader resolve manifest: %w", err)
	}
	arrow.Namespace = identity
	return arrow, nil
}

// heldPreview serves a refless ns from the manifest an earlier view of it
// settled on, which the vault marks as its default, whatever the age, so a
// restart or an expired snapshot never makes a view wait on the host. A pin
// discovery filed is not that: it names no channel, and a view of it still
// takes the live path. The host is asked behind the call.
func (r *storeService) heldPreview(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, bool) {
	if r.vault == nil || r.manifold == nil {
		return nil, false
	}
	refs, _ := r.vault.ListVersions(ctx, ns)
	for _, ref := range refs {
		arrow, file, ok := r.readHeld(ctx, ns.WithRef(ref))
		if !ok || !file.Default {
			continue
		}
		r.recheck(ctx, ns, file.Commit)
		return arrow, true
	}
	return nil, false
}

func (r *storeService) readHeld(
	ctx context.Context,
	key domain.Namespace,
) (*domain.Arrow, vault.ManifestFile, bool) {
	file, err := r.vault.GetArrow(ctx, key)
	if err != nil && !errors.Is(err, vault.ErrStale) {
		return nil, file, false
	}
	arrow, err := r.manifold.ParseArrow(file.Content)
	if err != nil || arrow == nil {
		return nil, file, false
	}
	arrow.Namespace = key
	if file.Commit != "" {
		arrow.Resolved = domain.Resolved{Ref: file.Ref, Commit: file.Commit, Fingerprint: file.Commit}
	}
	return arrow, file, true
}

// recheck resolves ns the way a view that held nothing would, behind the one
// that did, and reports the arrow when it now stands at another commit. The
// resolution files its result in the vault for the next view.
func (r *storeService) recheck(
	ctx context.Context,
	ns domain.Namespace,
	heldCommit string,
) {
	recheckCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recheckTimeout)
	r.rechecking.Add(1)
	go func() {
		defer r.rechecking.Done()
		defer cancel()
		identity, fresh, err := r.ResolveInstall(recheckCtx, ns, Preview())
		if err != nil {
			slog.WarnContext(recheckCtx, "store: recheck held preview", "ns", ns, "err", err)
			return
		}
		if r.onRefreshed != nil && fresh.Resolved.Commit != heldCommit {
			fresh.Namespace = identity
			r.onRefreshed(*fresh)
		}
	}()
}
