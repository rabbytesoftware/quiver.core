package vault

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type store struct {
	vaultPath      string
	namespacesPath string
	ttl            time.Duration
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
	}
	return NewWithClock(vaultPath, namespacesPath, ttl, time.Now)
}

func NewWithClock(
	vaultPath string,
	namespacesPath string,
	ttl time.Duration,
	clock func() time.Time,
) Vault {
	return &store{
		vaultPath:      vaultPath,
		namespacesPath: namespacesPath,
		ttl:            ttl,
		clock:          clock,
		locks:          make(map[string]*sync.Mutex),
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
	manifest *domain.QuiverManifest,
) (string, error) {
	if err := ns.Validate(); err != nil {
		return "", ErrInvalidNamespace
	}
	return putQuiver(s, ns, manifest)
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
