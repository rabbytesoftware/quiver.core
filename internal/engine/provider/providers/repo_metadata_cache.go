package providers

import (
	"context"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	repoMetadataFailureTTL = 10 * time.Minute
	repoMetadataMaxEntries = 4096
)

type repoMetadataEntry struct {
	ready    chan struct{}
	meta     domain.RepoMetadata
	err      error
	failedAt time.Time
}

// repoMetadataCache lets concurrent and repeated callers share one request
// per repository. A success is kept for the process; a failure is kept for a
// while, so a rate-limited host is not asked again for every draft.
type repoMetadataCache struct {
	mu      sync.Mutex
	entries map[string]*repoMetadataEntry
	now     func() time.Time
}

type repoMetadataLoad func(
	ctx context.Context,
) (domain.RepoMetadata, error)

func newRepoMetadataCache(
	now func() time.Time,
) *repoMetadataCache {
	return &repoMetadataCache{
		entries: make(map[string]*repoMetadataEntry),
		now:     now,
	}
}

func (c *repoMetadataCache) get(
	ctx context.Context,
	key string,
	load repoMetadataLoad,
) (domain.RepoMetadata, error) {
	entry, owner := c.claim(key)
	if owner {
		entry.meta, entry.err = load(context.WithoutCancel(ctx))
		if entry.err != nil {
			entry.failedAt = c.now()
		}
		close(entry.ready)
	}

	select {
	case <-entry.ready:
		return entry.meta, entry.err
	case <-ctx.Done():
		return domain.RepoMetadata{}, ctx.Err()
	}
}

func (c *repoMetadataCache) claim(
	key string,
) (*repoMetadataEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entries[key]; ok && !c.expired(entry) {
		return entry, false
	}
	if len(c.entries) >= repoMetadataMaxEntries {
		clear(c.entries)
	}
	entry := &repoMetadataEntry{ready: make(chan struct{})}
	c.entries[key] = entry
	return entry, true
}

func (c *repoMetadataCache) expired(
	entry *repoMetadataEntry,
) bool {
	select {
	case <-entry.ready:
	default:
		return false
	}
	return entry.err != nil && c.now().Sub(entry.failedAt) >= repoMetadataFailureTTL
}
