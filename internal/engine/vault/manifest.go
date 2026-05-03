package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func getArrow(s *store, ns domain.Namespace) (ManifestFile, error) {
	mu := s.namespaceLock(string(ns))
	mu.Lock()
	defer mu.Unlock()

	meta, err := readMeta(s.metaFilePath(ns))
	if errors.Is(err, os.ErrNotExist) {
		return ManifestFile{}, ErrNotCached
	}
	if err != nil {
		return ManifestFile{}, err
	}

	content, err := os.ReadFile(s.manifestFilePath(ns, meta.Filename)) // #nosec G304 -- path derived from URL-encoded namespace
	if errors.Is(err, os.ErrNotExist) {
		return ManifestFile{}, ErrNotCached
	}
	if err != nil {
		return ManifestFile{}, err
	}

	file := ManifestFile{Content: content, Filename: meta.Filename}

	if s.clock().Sub(meta.CachedAt) > s.ttl {
		return file, ErrStale
	}
	return file, nil
}

func putArrow(s *store, ns domain.Namespace, file ManifestFile) error {
	mu := s.namespaceLock(string(ns))
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(s.vaultPath, 0o700); err != nil {
		return err
	}

	// Write raw manifest verbatim.
	manifestPath := s.manifestFilePath(ns, file.Filename)
	if err := atomicWrite(manifestPath, file.Content); err != nil {
		return err
	}

	// Write meta sidecar.
	metaData, err := json.Marshal(VaultMetadata{
		CachedAt: s.clock(),
		Filename: file.Filename,
	})
	if err != nil {
		return err
	}
	if err := atomicWrite(s.metaFilePath(ns), metaData); err != nil {
		return err
	}

	// Create namespace workdir as a side effect.
	return os.MkdirAll(s.workdirPath(ns), 0o700)
}

func deleteArrow(s *store, ns domain.Namespace) error {
	mu := s.namespaceLock(string(ns))
	mu.Lock()
	defer mu.Unlock()

	meta, err := readMeta(s.metaFilePath(ns))
	if errors.Is(err, os.ErrNotExist) {
		return nil // idempotent
	}
	if err != nil {
		return err
	}

	if err := os.Remove(s.manifestFilePath(ns, meta.Filename)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("vault delete: remove manifest: %w", err)
	}
	if err := os.Remove(s.metaFilePath(ns)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("vault delete: remove meta: %w", err)
	}
	return nil
}

func renameArrow(s *store, oldNs, newNs domain.Namespace) error {
	// consistent ordering to prevent lock-order deadlock
	first, second := string(oldNs), string(newNs)
	if first > second {
		first, second = second, first
	}
	mu1 := s.namespaceLock(first)
	mu2 := s.namespaceLock(second)
	mu1.Lock()
	defer mu1.Unlock()
	mu2.Lock()
	defer mu2.Unlock()

	meta, err := readMeta(s.metaFilePath(oldNs))
	if err != nil {
		return fmt.Errorf("vault rename: read old meta: %w", err)
	}

	oldManifest := s.manifestFilePath(oldNs, meta.Filename)
	newManifest := s.manifestFilePath(newNs, meta.Filename)
	oldMeta := s.metaFilePath(oldNs)
	newMeta := s.metaFilePath(newNs)

	if err := os.Rename(oldManifest, newManifest); err != nil {
		return fmt.Errorf("vault rename manifest: %w", err)
	}
	if err := os.Rename(oldMeta, newMeta); err != nil {
		_ = os.Rename(newManifest, oldManifest) // rollback
		return fmt.Errorf("vault rename meta: %w", err)
	}
	return nil
}

func listVersions(s *store, ns domain.Namespace) ([]string, error) {
	bare := ns.BareNamespace()

	entries, err := os.ReadDir(s.vaultPath)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return []string{}, fmt.Errorf("vault list versions: %w", err)
	}

	var versions []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".meta.json") {
			continue
		}

		encoded := strings.TrimSuffix(e.Name(), ".meta.json")
		decoded, err := url.PathUnescape(encoded)
		if err != nil {
			continue
		}

		candidate := domain.Namespace(decoded)
		if candidate.BareNamespace() != bare {
			continue
		}

		if ref := candidate.Ref(); ref != "" {
			versions = append(versions, ref)
		}
	}

	if versions == nil {
		return []string{}, nil
	}
	return versions, nil
}

func readMeta(path string) (VaultMetadata, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- caller validates path
	if err != nil {
		return VaultMetadata{}, err
	}
	var m VaultMetadata
	if err := json.Unmarshal(data, &m); err != nil {
		return VaultMetadata{}, err
	}
	return m, nil
}

// atomicWrite writes data to path using a temp file + rename for atomicity.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "*.tmp") // #nosec G304 -- dir is caller-validated
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func acquireNamespace(s *store, ns domain.Namespace) (*sync.Mutex, string, error) {
	base := s.namespacesPath
	resolved := filepath.Clean(filepath.Join(base, filepath.FromSlash(ns.String())))
	if !strings.HasPrefix(resolved, base+string(filepath.Separator)) {
		return nil, "", ErrInvalidNamespace
	}
	return s.namespaceLock(ns.String()), resolved, nil
}

func getQuiver(s *store, ns domain.Namespace) (*QuiverVaultEntry, string, error) {
	mu, dir, err := acquireNamespace(s, ns)
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
		Manifest     *domain.QuiverManifest `json:"manifest"`
		FailedArrows []domain.Namespace     `json:"failed_arrows,omitempty"`
		CachedAt     time.Time              `json:"cached_at"`
	}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		return nil, "", err
	}

	entry := &QuiverVaultEntry{
		Manifest:     onDisk.Manifest,
		FailedArrows: onDisk.FailedArrows,
		Metadata:     VaultMetadata{CachedAt: onDisk.CachedAt},
	}

	if s.clock().Sub(onDisk.CachedAt) > s.ttl {
		return entry, path, ErrStale
	}
	return entry, path, nil
}

func putQuiver(
	s *store,
	ns domain.Namespace,
	manifest *domain.QuiverManifest,
	failedArrows []domain.Namespace,
) (string, error) {
	mu, dir, err := acquireNamespace(s, ns)
	if err != nil {
		return "", err
	}
	mu.Lock()
	defer mu.Unlock()

	path := filepath.Join(dir, quiverFilename)

	onDisk := struct {
		Manifest     *domain.QuiverManifest `json:"manifest"`
		FailedArrows []domain.Namespace     `json:"failed_arrows,omitempty"`
		CachedAt     time.Time              `json:"cached_at"`
	}{
		Manifest:     manifest,
		FailedArrows: failedArrows,
		CachedAt:     s.clock(),
	}

	data, err := json.Marshal(onDisk)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	if err := atomicWrite(path, data); err != nil {
		return "", err
	}
	return path, nil
}

func updateFailedArrows(
	s *store,
	ns domain.Namespace,
	failedArrows []domain.Namespace,
) error {
	mu, dir, err := acquireNamespace(s, ns)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()

	path := filepath.Join(dir, quiverFilename)
	data, err := os.ReadFile(path) // #nosec G304 -- path is sanitised by acquireNamespace()
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotCached
	}
	if err != nil {
		return err
	}

	var onDisk struct {
		Manifest     *domain.QuiverManifest `json:"manifest"`
		FailedArrows []domain.Namespace     `json:"failed_arrows,omitempty"`
		CachedAt     time.Time              `json:"cached_at"`
	}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		return err
	}

	onDisk.FailedArrows = failedArrows

	updated, err := json.Marshal(onDisk)
	if err != nil {
		return err
	}

	return atomicWrite(path, updated)
}

func listCachedQuivers(s *store) ([]domain.Namespace, error) {
	entries, err := os.ReadDir(s.namespacesPath)
	if errors.Is(err, os.ErrNotExist) {
		return []domain.Namespace{}, nil
	}
	if err != nil {
		return []domain.Namespace{}, fmt.Errorf("vault list quivers: %w", err)
	}

	var result []domain.Namespace
	for _, top := range entries {
		if !top.IsDir() {
			continue
		}
		found, err := findQuiversUnder(filepath.Join(s.namespacesPath, top.Name()), top.Name())
		if err != nil {
			return []domain.Namespace{}, err
		}
		result = append(result, found...)
	}

	if result == nil {
		return []domain.Namespace{}, nil
	}
	return result, nil
}

func findQuiversUnder(dir, relPath string) ([]domain.Namespace, error) {
	quiverPath := filepath.Join(dir, quiverFilename)
	if _, err := os.Stat(quiverPath); err == nil {
		ns := domain.Namespace(filepath.ToSlash(relPath))
		return []domain.Namespace{ns}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("vault list quivers: read dir %s: %w", dir, err)
	}

	var result []domain.Namespace
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		childRel := relPath + "/" + e.Name()
		found, err := findQuiversUnder(filepath.Join(dir, e.Name()), childRel)
		if err != nil {
			return nil, err
		}
		result = append(result, found...)
	}
	return result, nil
}

func deleteQuiver(s *store, ns domain.Namespace) error {
	mu, dir, err := acquireNamespace(s, ns)
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
	return nil
}
