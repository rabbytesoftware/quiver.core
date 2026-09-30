package vault

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// sweep runs both retention tiers: manifest bytes expire on a fixed clock from
// their write, index rows on a sliding clock reset by every re-save. A row that
// outlives its bytes is a supported state — GetArrow refetches.
//
// A closed index is not a sweep failure: the vault was shut down while this pass
// was already running, and the rows it would have evicted are gone with the
// process.
func (s *store) sweep() {
	s.sweepArrows()
	s.sweepQuivers()

	err := s.withIndex(func(i *index) error { return i.evictExpired(s.clock()) })
	if err != nil && !errors.Is(err, ErrClosed) {
		slog.WarnContext(context.Background(), "vault sweep: evict index rows", "err", err)
	}
}

func (s *store) sweepArrows() {
	entries, err := os.ReadDir(s.vaultPath)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), metaSuffix) {
			continue
		}
		meta, err := readMeta(filepath.Join(s.vaultPath, e.Name()))
		if err != nil || s.clock().Sub(meta.CachedAt) <= s.ttl {
			continue
		}
		s.sweepCacheEntry(strings.TrimSuffix(e.Name(), metaSuffix))
	}
}

// sweepCacheEntry removes one expired cache entry under its namespace's
// lock; an entry whose namespace cannot be read is left alone.
func (s *store) sweepCacheEntry(name string) {
	ns, ok := s.cachedNamespace(name + metaSuffix)
	if !ok {
		return
	}
	mu := s.namespaceLock(string(ns))
	mu.Lock()
	defer mu.Unlock()
	_ = deleteCacheEntry(s, name)
}

func (s *store) sweepQuivers() {
	_ = filepath.WalkDir(s.namespacesPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return filepath.SkipAll
			}
			return nil
		}
		if d.IsDir() || d.Name() != quiverFilename {
			return nil
		}
		cachedAt, err := readQuiverCachedAt(path)
		if err != nil || s.clock().Sub(cachedAt) <= s.ttl {
			return nil
		}
		rel, err := filepath.Rel(s.namespacesPath, filepath.Dir(path))
		if err != nil {
			return nil
		}
		_ = deleteCollection(s, collectionNamespace(filepath.ToSlash(rel), path))
		return nil
	})
}

// readQuiverCachedAt reads only the cached_at timestamp from a quiver.json file.
func readQuiverCachedAt(path string) (time.Time, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path comes from WalkDir under namespacesPath
	if err != nil {
		return time.Time{}, err
	}
	var v struct {
		CachedAt time.Time `json:"cached_at"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return time.Time{}, err
	}
	return v.CachedAt, nil
}
