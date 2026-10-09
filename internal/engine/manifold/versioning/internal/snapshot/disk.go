package snapshot

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"path/filepath"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type stored struct {
	Snapshot domain.RefSnapshot `json:"snapshot"`
	CachedAt time.Time          `json:"cached_at"`
}

// disk keeps each repository's last snapshot across restarts. An empty dir
// keeps nothing.
type disk struct {
	dir string
}

func (d disk) path(
	bare domain.Namespace,
) string {
	return filepath.Join(d.dir, url.PathEscape(string(bare))+".json")
}

func (d disk) load(
	ctx context.Context,
	bare domain.Namespace,
) (entry, bool) {
	if d.dir == "" {
		return entry{}, false
	}
	raw, err := fns.Read(ctx, d.path(bare))
	if err != nil {
		return entry{}, false
	}
	var s stored
	if err := json.Unmarshal(raw, &s); err != nil {
		return entry{}, false
	}
	return entry{snap: s.Snapshot, cachedAt: s.CachedAt}, true
}

func (d disk) save(
	ctx context.Context,
	bare domain.Namespace,
	e entry,
) {
	if d.dir == "" {
		return
	}
	raw, err := json.Marshal(stored{Snapshot: e.snap, CachedAt: e.cachedAt})
	if err != nil {
		return
	}
	if err := fns.MkdirAll(ctx, d.dir, 0o700); err != nil {
		slog.WarnContext(ctx, "snapshot: create refs dir", "dir", d.dir, "err", err)
		return
	}
	if err := fns.Write(ctx, d.path(bare), raw); err != nil {
		slog.WarnContext(ctx, "snapshot: save refs", "ns", bare, "err", err)
	}
}
