package vault

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

// getArrow retrieves an arrow entry from disk.
// Returns ErrNotCached if not found, ErrStale if TTL expired.
// On ErrStale, both entry and path are returned.
func getArrow(
	s *store,
	ns domain.Namespace,
) (*VaultEntry, string, error) {
	mu, dir, err := s.acquireNamespace(ns)
	if err != nil {
		return nil, "", err
	}
	mu.Lock()
	defer mu.Unlock()

	path := filepath.Join(dir, arrowFilename)
	data, err := os.ReadFile(path) // #nosec G304 -- path is sanitised by acquireNamespace()
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", ErrNotCached
	}
	if err != nil {
		return nil, "", err
	}

	var onDisk struct {
		Manifest             *domain.ArrowManifest `json:"manifest"`
		CachedAt             time.Time             `json:"cached_at"`
		OS                   string                `json:"os"`
		IndirectDependencies []domain.Namespace    `json:"indirect_dependencies,omitempty"`
	}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		return nil, "", err
	}

	entry := &VaultEntry{
		Manifest: onDisk.Manifest,
		Metadata: VaultMetadata{
			CachedAt: onDisk.CachedAt,
			OS:       onDisk.OS,
		},
		IndirectDependencies: onDisk.IndirectDependencies,
	}

	if time.Since(onDisk.CachedAt) > s.ttl {
		return entry, path, ErrStale
	}
	return entry, path, nil
}

// getQuiver retrieves a quiver entry from disk.
// Returns ErrNotCached if not found, ErrStale if TTL expired.
// On ErrStale, both entry and path are returned.
func getQuiver(
	s *store,
	ns domain.Namespace,
) (*QuiverVaultEntry, string, error) {
	mu, dir, err := s.acquireNamespace(ns)
	if err != nil {
		return nil, "", err
	}
	mu.Lock()
	defer mu.Unlock()

	path := filepath.Join(dir, quiverFilename)
	data, err := os.ReadFile(path) // #nosec G304 -- path is sanitised by acquireNamespace()
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", ErrNotCached
	}
	if err != nil {
		return nil, "", err
	}

	var onDisk struct {
		Manifest  *domain.QuiverManifest `json:"manifest"`
		CachedAt  time.Time              `json:"cached_at"`
		OS        string                 `json:"os"`
	}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		return nil, "", err
	}

	entry := &QuiverVaultEntry{
		Manifest: onDisk.Manifest,
		Metadata: VaultMetadata{
			CachedAt: onDisk.CachedAt,
			OS:       onDisk.OS,
		},
	}

	if time.Since(onDisk.CachedAt) > s.ttl {
		return entry, path, ErrStale
	}
	return entry, path, nil
}

// putArrow persists an arrow manifest with optional indirect dependencies.
// Acquires the per-namespace lock for atomic write safety.
func putArrow(
	s *store,
	ns domain.Namespace,
	manifest *domain.ArrowManifest,
	indirectDeps []domain.Namespace,
) (string, error) {
	mu, dir, err := s.acquireNamespace(ns)
	if err != nil {
		return "", err
	}
	mu.Lock()
	defer mu.Unlock()

	path := filepath.Join(dir, arrowFilename)

	onDisk := struct {
		Manifest             *domain.ArrowManifest `json:"manifest"`
		CachedAt             time.Time             `json:"cached_at"`
		OS                   string                `json:"os"`
		IndirectDependencies []domain.Namespace    `json:"indirect_dependencies,omitempty"`
	}{
		Manifest:             manifest,
		CachedAt:             time.Now(),
		OS:                   s.osVersion,
		IndirectDependencies: indirectDeps,
	}

	data, err := json.Marshal(onDisk)
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

// putQuiver persists a quiver manifest.
// Acquires the per-namespace lock for atomic write safety.
func putQuiver(
	s *store,
	ns domain.Namespace,
	manifest *domain.QuiverManifest,
) (string, error) {
	mu, dir, err := s.acquireNamespace(ns)
	if err != nil {
		return "", err
	}
	mu.Lock()
	defer mu.Unlock()

	path := filepath.Join(dir, quiverFilename)

	onDisk := struct {
		Manifest *domain.QuiverManifest `json:"manifest"`
		CachedAt time.Time              `json:"cached_at"`
		OS       string                 `json:"os"`
	}{
		Manifest: manifest,
		CachedAt: time.Now(),
		OS:       s.osVersion,
	}

	data, err := json.Marshal(onDisk)
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

// deleteArrow removes arrow.json and, if quiver.json doesn't exist, removes the directory.
// Idempotent — returns nil if arrow.json does not exist.
func deleteArrow(
	s *store,
	ns domain.Namespace,
) error {
	mu, dir, err := s.acquireNamespace(ns)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()

	arrowPath := filepath.Join(dir, arrowFilename)
	err = os.Remove(arrowPath) // #nosec G304 -- path is sanitised by acquireNamespace()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Check if quiver.json exists; if not, remove the directory.
	quiverPath := filepath.Join(dir, quiverFilename)
	if _, err := os.Stat(quiverPath); errors.Is(err, os.ErrNotExist) {
		// Quiver doesn't exist; safe to remove the directory.
		_ = os.RemoveAll(dir) // #nosec G304 -- dir is sanitised by acquireNamespace()
	}

	return nil
}

// deleteQuiver removes quiver.json and, if arrow.json doesn't exist, removes the directory.
// Idempotent — returns nil if quiver.json does not exist.
func deleteQuiver(
	s *store,
	ns domain.Namespace,
) error {
	mu, dir, err := s.acquireNamespace(ns)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()

	quiverPath := filepath.Join(dir, quiverFilename)
	err = os.Remove(quiverPath) // #nosec G304 -- path is sanitised by acquireNamespace()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Check if arrow.json exists; if not, remove the directory.
	arrowPath := filepath.Join(dir, arrowFilename)
	if _, err := os.Stat(arrowPath); errors.Is(err, os.ErrNotExist) {
		// Arrow doesn't exist; safe to remove the directory.
		_ = os.RemoveAll(dir) // #nosec G304 -- dir is sanitised by acquireNamespace()
	}

	return nil
}
