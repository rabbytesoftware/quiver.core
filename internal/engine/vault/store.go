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
	locks     sync.Map
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
	}
}

func (s *store) namespaceLock(
	ns domain.Namespace,
) *sync.Mutex {
	mu := &sync.Mutex{}
	actual, _ := s.locks.LoadOrStore(ns.String(), mu)
	return actual.(*sync.Mutex)
}

func (s *store) namespacePath(
	ns domain.Namespace,
) (string, error) {
	base := filepath.Join(s.basePath, "namespaces")
	resolved := filepath.Clean(filepath.Join(base, ns.String()))
	if !strings.HasPrefix(resolved, base+string(filepath.Separator)) {
		return "", ErrInvalidNamespace
	}
	return resolved, nil
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
