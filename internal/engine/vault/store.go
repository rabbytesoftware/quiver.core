package vault

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type store struct {
	vaultPath      string
	namespacesPath string
	ttl            time.Duration
	sweepInterval  time.Duration
	clock          func() time.Time
	mu             sync.RWMutex
	locks          map[string]*sync.Mutex
}

func New(
	vaultPath string,
	namespacesPath string,
	ttl time.Duration,
) Vault {
	if vaultPath == "" {
		vaultPath = metadata.GetVaultPath()
	}
	if namespacesPath == "" {
		namespacesPath = metadata.GetNamespacesPath()
	}
	if ttl == 0 {
		ttl = 24 * time.Hour
		if d, err := time.ParseDuration(config.GetVault().TTL); err == nil && d > 0 {
			ttl = d
		}
	}
	sweepInterval := 5 * time.Minute
	if d, err := time.ParseDuration(config.GetVault().SweepInterval); err == nil && d > 0 {
		sweepInterval = d
	}
	return newStore(vaultPath, namespacesPath, ttl, sweepInterval, time.Now)
}

func NewWithClock(
	vaultPath string,
	namespacesPath string,
	ttl time.Duration,
	clock func() time.Time,
) Vault {
	return newStore(vaultPath, namespacesPath, ttl, 5*time.Minute, clock)
}

func newStore(
	vaultPath string,
	namespacesPath string,
	ttl time.Duration,
	sweepInterval time.Duration,
	clock func() time.Time,
) Vault {
	return &store{
		vaultPath:      vaultPath,
		namespacesPath: namespacesPath,
		ttl:            ttl,
		sweepInterval:  sweepInterval,
		clock:          clock,
		locks:          make(map[string]*sync.Mutex),
	}
}

func (s *store) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.sweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sweep()
			}
		}
	}()
}

// namespaceLock returns the per-namespace mutex, creating it on first access.
func (s *store) namespaceLock(key string) *sync.Mutex {
	s.mu.RLock()
	if m, ok := s.locks[key]; ok {
		s.mu.RUnlock()
		return m
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.locks[key]; ok {
		return m
	}
	m := &sync.Mutex{}
	s.locks[key] = m
	return m
}

// encodeNS URL-encodes a namespace so it is safe to use as a flat filename.
func encodeNS(ns domain.Namespace) string {
	return url.PathEscape(string(ns))
}

func (s *store) metaFilePath(ns domain.Namespace) string {
	return filepath.Join(s.vaultPath, encodeNS(ns)+".meta.json")
}

func (s *store) manifestFilePath(ns domain.Namespace, filename string) string {
	ext := filepath.Ext(filename)
	return filepath.Join(s.vaultPath, encodeNS(ns)+ext)
}

func (s *store) workdirPath(ns domain.Namespace) string {
	return filepath.Join(s.namespacesPath, filepath.FromSlash(string(ns)))
}

func (s *store) WorkDir(
	_ context.Context,
	ns domain.Namespace,
) (string, error) {
	if err := ns.Validate(); err != nil {
		return "", ErrInvalidNamespace
	}
	dir := s.workdirPath(ns)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("workdir %s: %w", ns, err)
	}
	return dir, nil
}

func (s *store) GetArrow(
	ctx context.Context,
	ns domain.Namespace,
) (ManifestFile, error) {
	if err := ns.Validate(); err != nil {
		return ManifestFile{}, ErrInvalidNamespace
	}
	return getArrow(s, ns)
}

func (s *store) GetQuiver(
	ctx context.Context,
	ns domain.Namespace,
) (*QuiverVaultEntry, string, error) {
	if err := ns.Validate(); err != nil {
		return nil, "", ErrInvalidNamespace
	}
	return getQuiver(s, ns)
}

func (s *store) PutArrow(
	ctx context.Context,
	ns domain.Namespace,
	file ManifestFile,
) error {
	if err := ns.Validate(); err != nil {
		return ErrInvalidNamespace
	}
	return putArrow(s, ns, file)
}

func (s *store) PutQuiver(
	ctx context.Context,
	ns domain.Namespace,
	quiver *domain.Quiver,
) (string, error) {
	if err := ns.Validate(); err != nil {
		return "", ErrInvalidNamespace
	}
	return putQuiver(s, ns, quiver)
}

func (s *store) ListCachedQuivers(_ context.Context) ([]domain.Namespace, error) {
	return listCachedQuivers(s)
}

func (s *store) DeleteArrow(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if err := ns.Validate(); err != nil {
		return ErrInvalidNamespace
	}
	return deleteArrow(s, ns)
}

func (s *store) DeleteWorkDir(
	_ context.Context,
	ns domain.Namespace,
) error {
	if err := ns.Validate(); err != nil {
		return ErrInvalidNamespace
	}
	dir := s.workdirPath(ns)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("workdir delete %s: %w", ns, err)
	}
	// Prune empty parent directories up to (but not including) namespacesPath.
	// macOS Finder metadata files (.DS_Store, ._*) are removed before the
	// emptiness check so they don't block pruning on macOS.
	for parent := filepath.Dir(dir); parent != s.namespacesPath; parent = filepath.Dir(parent) {
		removeMacOSMetadata(parent)
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) > 0 {
			break
		}
		if err := os.Remove(parent); err != nil {
			break
		}
	}
	return nil
}

func (s *store) DeleteQuiver(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if err := ns.Validate(); err != nil {
		return ErrInvalidNamespace
	}
	return deleteQuiver(s, ns)
}

func (s *store) RenameArrow(
	ctx context.Context,
	oldNs domain.Namespace,
	newNs domain.Namespace,
) error {
	if err := oldNs.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidNamespace, err)
	}
	if err := newNs.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidNamespace, err)
	}
	if oldNs == newNs {
		return nil
	}
	return renameArrow(s, oldNs, newNs)
}

func (s *store) ListVersions(
	ctx context.Context,
	ns domain.Namespace,
) ([]string, error) {
	if err := ns.BareNamespace().Validate(); err != nil {
		return []string{}, nil
	}
	return listVersions(s, ns)
}

// removeMacOSMetadata deletes Finder-created metadata files (.DS_Store, ._*)
// from dir so they don't block empty-directory detection during pruning.
func removeMacOSMetadata(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if name == ".DS_Store" || (len(name) > 2 && name[:2] == "._") {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}
