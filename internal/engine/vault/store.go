package vault

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type store struct {
	basePath string
	ttl      time.Duration
	clock    func() time.Time
	mu       sync.RWMutex
	locks    map[string]*sync.Mutex
}

func New(
	basePath string,
	ttl time.Duration,
) Vault {
	if basePath == "" {
		// Uses GetNamespacesPath directly (not paths.Namespaces) because the vault
		// creates per-namespace subdirectories lazily on first write via os.MkdirAll
		// in manifest.go, which also creates the parent namespaces directory.
		basePath = metadata.GetNamespacesPath()
	}
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	return &store{
		basePath: basePath,
		ttl:      ttl,
		clock:    time.Now,
		mu:       sync.RWMutex{},
		locks:    make(map[string]*sync.Mutex),
	}
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

// acquireNamespace validates the namespace path and returns the per-namespace
// mutex (unlocked) and the resolved directory path.
func (s *store) acquireNamespace(ns domain.Namespace) (*sync.Mutex, string, error) {
	base := s.basePath
	resolved := filepath.Clean(filepath.Join(base, ns.String()))
	if !strings.HasPrefix(resolved, base+string(filepath.Separator)) {
		return nil, "", ErrInvalidNamespace
	}
	return s.namespaceLock(ns.String()), resolved, nil
}

func (s *store) GetArrow(
	ctx context.Context,
	namespace domain.Namespace,
) (*VaultEntry, string, error) {
	if err := namespace.Validate(); err != nil {
		return nil, "", ErrInvalidNamespace
	}
	return getArrow(s, namespace)
}

func (s *store) GetQuiver(
	ctx context.Context,
	namespace domain.Namespace,
) (*QuiverVaultEntry, string, error) {
	if err := namespace.Validate(); err != nil {
		return nil, "", ErrInvalidNamespace
	}
	return getQuiver(s, namespace)
}

func (s *store) PutArrow(
	ctx context.Context,
	namespace domain.Namespace,
	manifest *domain.Arrow,
) (string, error) {
	if err := namespace.Validate(); err != nil {
		return "", ErrInvalidNamespace
	}
	return putArrow(s, namespace, manifest)
}

func (s *store) PutQuiver(
	ctx context.Context,
	namespace domain.Namespace,
	manifest *domain.QuiverManifest,
) (string, error) {
	if err := namespace.Validate(); err != nil {
		return "", ErrInvalidNamespace
	}
	return putQuiver(s, namespace, manifest)
}

func (s *store) DeleteArrow(
	ctx context.Context,
	namespace domain.Namespace,
) error {
	if err := namespace.Validate(); err != nil {
		return ErrInvalidNamespace
	}
	return deleteArrow(s, namespace)
}

func (s *store) DeleteQuiver(
	ctx context.Context,
	namespace domain.Namespace,
) error {
	if err := namespace.Validate(); err != nil {
		return ErrInvalidNamespace
	}
	return deleteQuiver(s, namespace)
}

func (s *store) ListVersions(
	ctx context.Context,
	namespace domain.Namespace,
) ([]string, error) {
	if err := namespace.BareNamespace().Validate(); err != nil {
		return []string{}, nil
	}
	return listVersions(s, namespace)
}
