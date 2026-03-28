package vault

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type store struct {
	basePath  string
	ttl       time.Duration
	osVersion string
	mu        sync.RWMutex
	locks     map[string]*sync.Mutex
}

func New(
	basePath string,
	ttl time.Duration,
	osVersion string,
) Vault {
	return &store{
		basePath:  basePath,
		ttl:       ttl,
		osVersion: osVersion,
		locks:     make(map[string]*sync.Mutex),
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
	base := filepath.Join(s.basePath, "namespaces")
	resolved := filepath.Clean(filepath.Join(base, ns.String()))
	if !strings.HasPrefix(resolved, base+string(filepath.Separator)) {
		return nil, "", ErrInvalidNamespace
	}
	return s.namespaceLock(ns.String()), resolved, nil
}

func (s *store) GetArrow(
	namespace domain.Namespace,
) (*domain.ArrowManifest, string, error) {
	if err := namespace.Validate(); err != nil {
		return nil, "", ErrInvalidNamespace
	}
	return getManifest[domain.ArrowManifest](s, namespace, arrowFilename)
}

func (s *store) GetQuiver(
	namespace domain.Namespace,
) (*domain.QuiverManifest, string, error) {
	if err := namespace.Validate(); err != nil {
		return nil, "", ErrInvalidNamespace
	}
	return getManifest[domain.QuiverManifest](s, namespace, quiverFilename)
}

func (s *store) PutArrow(
	namespace domain.Namespace,
	manifest *domain.ArrowManifest,
) (string, error) {
	if err := namespace.Validate(); err != nil {
		return "", ErrInvalidNamespace
	}
	return putManifest(s, namespace, arrowFilename, manifest)
}

func (s *store) PutQuiver(
	namespace domain.Namespace,
	manifest *domain.QuiverManifest,
) (string, error) {
	if err := namespace.Validate(); err != nil {
		return "", ErrInvalidNamespace
	}
	return putManifest(s, namespace, quiverFilename, manifest)
}

func (s *store) DeleteArrow(
	namespace domain.Namespace,
) error {
	if err := namespace.Validate(); err != nil {
		return ErrInvalidNamespace
	}
	return deleteManifest(s, namespace, arrowFilename)
}

func (s *store) DeleteQuiver(
	namespace domain.Namespace,
) error {
	if err := namespace.Validate(); err != nil {
		return ErrInvalidNamespace
	}
	return deleteManifest(s, namespace, quiverFilename)
}
