package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
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

// heldPreview serves a refless ns from what the vault already holds for its
// repository, whatever the age, so neither a restart nor an arrow search has
// already shown makes a view wait on the host. The host is asked behind the
// call, and a view that answered from a build discovery filed learns its
// channel there.
func (r *storeService) heldPreview(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, bool) {
	arrow, file, ok := r.heldDefault(ctx, ns)
	if !ok {
		return nil, false
	}
	r.recheck(ctx, ns, file.Commit)
	return arrow, true
}

// HeldChannels is the channel list the vault filed with ns's default manifest,
// when there is one; the host is asked behind it, which files a fresh list.
func (r *storeService) HeldChannels(
	ctx context.Context,
	ns domain.Namespace,
) ([]manifold.ChannelInfo, bool) {
	_, file, ok := r.heldDefault(ctx, ns.BareNamespace())
	if !ok || len(file.Channels) == 0 {
		return nil, false
	}
	var channels []manifold.ChannelInfo
	if err := json.Unmarshal(file.Channels, &channels); err != nil {
		return nil, false
	}
	r.recheck(ctx, ns.BareNamespace(), file.Commit)
	return channels, true
}

// heldDefault is the manifest an earlier view of the refless ns settled on (the
// vault marks it as the default), else the first build discovery filed under a
// release tag. A repository that switched channels leaves a mark on each, so
// the newest one wins.
func (r *storeService) heldDefault(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, vault.ManifestFile, bool) {
	if r.vault == nil || r.manifold == nil {
		return nil, vault.ManifestFile{}, false
	}
	refs, _ := r.vault.ListVersions(ctx, ns)
	var best, filed *domain.Arrow
	var bestFile, filedFile vault.ManifestFile
	for _, ref := range refs {
		arrow, file, ok := r.readHeld(ctx, ns.WithRef(ref))
		if !ok {
			continue
		}
		if file.Default && (best == nil || file.CachedAt.After(bestFile.CachedAt)) {
			best, bestFile = arrow, file
		}
		if filed == nil {
			filed, filedFile = arrow, file
		}
	}
	if best != nil {
		return best, bestFile, true
	}
	return filed, filedFile, filed != nil
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
	stopOnShutdown := context.AfterFunc(r.recheckStop, cancel)
	r.rechecking.Add(1)
	go func() {
		defer r.rechecking.Done()
		defer cancel()
		defer stopOnShutdown()
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
