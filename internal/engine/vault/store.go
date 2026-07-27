package vault

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	defaultTTL      = 24 * time.Hour
	defaultIndexTTL = 720 * time.Hour
	indexFilename   = "index.db"
)

type store struct {
	vaultPath      string
	namespacesPath string
	ttl            time.Duration
	indexTTL       time.Duration
	sweepInterval  time.Duration
	clock          func() time.Time
	idx            *index
	idxMu          sync.Mutex
	closed         bool
	mu             sync.RWMutex
	locks          map[string]*sync.Mutex
}

func New(
	vaultPath string,
	namespacesPath string,
	ttl time.Duration,
) (Vault, error) {
	if vaultPath == "" {
		vaultPath = metadata.GetVaultPath()
	}
	if namespacesPath == "" {
		namespacesPath = metadata.GetNamespacesPath()
	}
	if ttl == 0 {
		ttl = defaultTTL
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
) (Vault, error) {
	return newStore(vaultPath, namespacesPath, ttl, 5*time.Minute, clock)
}

func newStore(
	vaultPath string,
	namespacesPath string,
	ttl time.Duration,
	sweepInterval time.Duration,
	clock func() time.Time,
) (Vault, error) {
	// The vault directory must exist before SQLite can create a file in it.
	if err := os.MkdirAll(vaultPath, 0o700); err != nil {
		return nil, fmt.Errorf("vault: create dir: %w", err)
	}

	idx, err := openIndex(filepath.Join(vaultPath, indexFilename))
	if err != nil {
		return nil, fmt.Errorf("vault: %w", err)
	}

	return &store{
		vaultPath:      vaultPath,
		namespacesPath: namespacesPath,
		ttl:            ttl,
		indexTTL:       resolveIndexTTL(),
		sweepInterval:  sweepInterval,
		clock:          clock,
		idx:            idx,
		locks:          make(map[string]*sync.Mutex),
	}, nil
}

func resolveIndexTTL() time.Duration {
	ttl := defaultIndexTTL
	if d, err := time.ParseDuration(config.GetVault().IndexTTL); err == nil && d > 0 {
		ttl = d
	}
	return ttl
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

// Close releases the index database. It is idempotent: the daemon closes the
// vault once, but a constructor that failed downstream discards it too, and the
// two must be able to overlap without double-closing a handle.
//
// The sweep goroutine is not stopped here — it exits with the context Start was
// given, which the daemon cancels before shutting anything down. A sweep that
// lands afterwards finds the index closed and leaves it alone.
func (s *store) Close() error {
	s.idxMu.Lock()
	defer s.idxMu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	return s.idx.close()
}

// withIndex runs fn under the lifecycle lock, so Close can never land while a
// connection is checked out and an in-flight query can never be handed a handle
// that is already gone. A closed vault reports ErrClosed rather than driving a
// dead database.
func (s *store) withIndex(fn func(i *index) error) error {
	s.idxMu.Lock()
	defer s.idxMu.Unlock()

	if s.closed {
		return ErrClosed
	}
	return fn(s.idx)
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

func (s *store) GetCollection(
	ctx context.Context,
	ns domain.Namespace,
) (*CollectionVaultEntry, string, error) {
	if err := ns.Validate(); err != nil {
		return nil, "", ErrInvalidNamespace
	}
	return getCollection(s, ns)
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

func (s *store) PutCollection(
	ctx context.Context,
	ns domain.Namespace,
	quiver *domain.Collection,
) (string, error) {
	if err := ns.Validate(); err != nil {
		return "", ErrInvalidNamespace
	}
	return putCollection(s, ns, quiver)
}

func (s *store) ListCachedCollections(_ context.Context) ([]domain.Namespace, error) {
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

func (s *store) DeleteCollection(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if err := ns.Validate(); err != nil {
		return ErrInvalidNamespace
	}
	return deleteCollection(s, ns)
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

func (s *store) SearchArrows(
	_ context.Context,
	q IndexQuery,
) ([]IndexRow, error) {
	var rows []IndexRow
	err := s.withIndex(func(i *index) error {
		var searchErr error
		rows, searchErr = i.search(q, s.clock())
		return searchErr
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *store) ForgetArrow(
	_ context.Context,
	ns domain.Namespace,
) error {
	if err := ns.Validate(); err != nil {
		return ErrInvalidNamespace
	}
	return s.withIndex(func(i *index) error { return i.forget(ns) })
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
