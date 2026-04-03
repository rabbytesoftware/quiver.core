package vault

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func getManifest[T any](
	s *store,
	ns domain.Namespace,
	filename string,
) (*T, string, error) {
	mu, dir, err := s.acquireNamespace(ns)
	if err != nil {
		return nil, "", err
	}
	mu.Lock()
	defer mu.Unlock()

	path := filepath.Join(dir, filename)
	data, err := os.ReadFile(path) // #nosec G304 -- path is sanitised by acquireNamespace()
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", ErrNotCached
	}
	if err != nil {
		return nil, "", err
	}
	var entry vaultEntry[T]
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, "", err
	}
	if time.Since(entry.CachedAt) > s.ttl {
		return &entry.Manifest, path, ErrStale
	}
	return &entry.Manifest, path, nil
}

func putManifest[T any](
	s *store,
	ns domain.Namespace,
	filename string,
	manifest *T,
) (string, error) {
	mu, dir, err := s.acquireNamespace(ns)
	if err != nil {
		return "", err
	}
	mu.Lock()
	defer mu.Unlock()

	path := filepath.Join(dir, filename)

	entry := vaultEntry[T]{
		CachedAt: time.Now(),
		OS:       s.osVersion,
		Manifest: *manifest,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil { // #nosec G304 -- dir is sanitised by acquireNamespace()
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "*.json") // #nosec G304 -- dir is sanitised by acquireNamespace()
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close() // #nosec G307 -- error is checked below
	if writeErr != nil {
		_ = os.Remove(tmpPath) // #nosec G104 -- best-effort cleanup of temp file
		return "", writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath) // #nosec G104 -- best-effort cleanup of temp file
		return "", closeErr
	}
	if err := os.Rename(tmpPath, path); err != nil { // #nosec G304 -- path is sanitised by acquireNamespace()
		_ = os.Remove(tmpPath) // #nosec G104 -- best-effort cleanup of temp file
		return "", err
	}
	return path, nil
}

func deleteManifest(
	s *store,
	ns domain.Namespace,
	filename string,
) error {
	mu, dir, err := s.acquireNamespace(ns)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()

	path := filepath.Join(dir, filename)
	err = os.Remove(path) // #nosec G304 -- path is sanitised by acquireNamespace()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
