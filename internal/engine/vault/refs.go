package vault

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const refsSuffix = ".refs.json"

func (s *store) refsFilePath(
	bare domain.Namespace,
) string {
	return filepath.Join(s.vaultPath, encodeNS(bare)+refsSuffix)
}

func (s *store) GetRefs(
	_ context.Context,
	ns domain.Namespace,
) (RefsEntry, error) {
	bare := ns.BareNamespace()
	if err := bare.Validate(); err != nil {
		return RefsEntry{}, ErrInvalidNamespace
	}
	mu := s.namespaceLock(string(bare))
	mu.Lock()
	defer mu.Unlock()

	raw, err := os.ReadFile(s.refsFilePath(bare)) // #nosec G304 -- path derived from URL-encoded namespace
	if errors.Is(err, os.ErrNotExist) {
		return RefsEntry{}, ErrNotCached
	}
	if err != nil {
		return RefsEntry{}, err
	}
	var entry RefsEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return RefsEntry{}, ErrNotCached
	}
	return entry, nil
}

func (s *store) PutRefs(
	_ context.Context,
	ns domain.Namespace,
	snap domain.RefSnapshot,
) error {
	bare := ns.BareNamespace()
	if err := bare.Validate(); err != nil {
		return ErrInvalidNamespace
	}
	raw, err := json.Marshal(RefsEntry{Snapshot: snap, CachedAt: s.clock()})
	if err != nil {
		return err
	}
	mu := s.namespaceLock(string(bare))
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(s.vaultPath, 0o700); err != nil {
		return err
	}
	return atomicWrite(s.refsFilePath(bare), raw)
}
