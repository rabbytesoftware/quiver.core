package vault

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func (s *store) sweep() {
	s.sweepArrows()
	s.sweepQuivers()
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
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".meta.json") {
			continue
		}
		encoded := strings.TrimSuffix(e.Name(), ".meta.json")
		decoded, err := url.PathUnescape(encoded)
		if err != nil {
			continue
		}
		meta, err := readMeta(filepath.Join(s.vaultPath, e.Name()))
		if err != nil || s.clock().Sub(meta.CachedAt) <= s.ttl {
			continue
		}
		_ = deleteArrow(s, domain.Namespace(decoded))
	}
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
		_ = deleteQuiver(s, domain.Namespace(filepath.ToSlash(rel)))
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
