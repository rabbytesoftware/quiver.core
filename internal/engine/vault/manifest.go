package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type quiverOnDisk struct {
	Collection *domain.Collection `json:"collection"`
	CachedAt   time.Time          `json:"cached_at"`
}

func getArrow(s *store, ns domain.Namespace) (ManifestFile, error) {
	mu := s.namespaceLock(string(ns))
	mu.Lock()
	defer mu.Unlock()

	meta, name, err := readCachedMeta(s, ns)
	if errors.Is(err, os.ErrNotExist) {
		return ManifestFile{}, ErrNotCached
	}
	if err != nil {
		return ManifestFile{}, err
	}

	if meta.NotFound {
		if s.clock().Sub(meta.CachedAt) > s.ttl {
			return ManifestFile{}, ErrNotCached
		}
		return ManifestFile{Commit: meta.Commit}, ErrConfirmedAbsent
	}

	content, err := os.ReadFile(filepath.Join(s.vaultPath, name+filepath.Ext(meta.Filename))) // #nosec G304 -- path derived from URL-encoded namespace
	if errors.Is(err, os.ErrNotExist) {
		return ManifestFile{}, ErrNotCached
	}
	if err != nil {
		return ManifestFile{}, err
	}

	file := ManifestFile{Content: content, Filename: meta.Filename, Ref: meta.Ref, Commit: meta.Commit, Default: meta.Default, Channels: meta.Channels, DefaultAt: meta.DefaultAt}

	if s.clock().Sub(meta.CachedAt) > s.ttl {
		return file, ErrStale
	}
	return file, nil
}

// putArrowNotFound records that ns's manifest was resolved live and
// definitively does not exist — the fetch succeeded well enough to clone
// the repository and inspect its tree, and none of the candidate filenames
// were there. Unlike putArrow, there is no manifest content to cache: only
// the meta sidecar is written, and no workdir is created, since there is
// nothing to build a workdir for. Subject to the same TTL as a positive
// entry (see getArrow), so a namespace whose ref later gains a manifest —
// possible for a branch, though not for an immutable tag — is eventually
// re-checked.
// readCachedMeta reads ns's meta under the first cache name that has one,
// and returns that name.
func readCachedMeta(s *store, ns domain.Namespace) (VaultMetadata, string, error) {
	err := os.ErrNotExist
	for _, name := range cacheNames(ns) {
		var meta VaultMetadata
		meta, err = readMeta(filepath.Join(s.vaultPath, name+metaSuffix))
		if !errors.Is(err, os.ErrNotExist) {
			return meta, name, err
		}
	}
	return VaultMetadata{}, "", err
}

func putArrowNotFound(s *store, ns domain.Namespace, commit string) error {
	mu := s.namespaceLock(string(ns))
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(s.vaultPath, 0o700); err != nil {
		return err
	}

	metaData, err := json.Marshal(VaultMetadata{
		CachedAt:  s.clock(),
		NotFound:  true,
		Namespace: ns,
		Commit:    commit,
	})
	if err != nil {
		return err
	}
	return atomicWrite(s.metaFilePath(ns), metaData)
}

func putArrow(s *store, ns domain.Namespace, file ManifestFile) error {
	mu := s.namespaceLock(string(ns))
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(s.vaultPath, 0o700); err != nil {
		return err
	}

	workdir, err := s.namespacePath(ns)
	if err != nil {
		return err
	}

	// Write raw manifest verbatim.
	manifestPath := s.manifestFilePath(ns, file.Filename)
	if err := atomicWrite(manifestPath, file.Content); err != nil {
		return err
	}

	file, defaultAt := keepPrevious(s, ns, file)

	// Write meta sidecar.
	metaData, err := json.Marshal(VaultMetadata{
		CachedAt:  s.clock(),
		Filename:  file.Filename,
		Namespace: ns,
		Ref:       file.Ref,
		Commit:    file.Commit,
		Default:   file.Default,
		Channels:  file.Channels,
		DefaultAt: defaultAt,
	})
	if err != nil {
		return err
	}
	if err := atomicWrite(s.metaFilePath(ns), metaData); err != nil {
		return err
	}

	if file.Meta != nil {
		if err := s.withIndex(func(i *index) error {
			return i.upsert(ns, file, *file.Meta, s.clock(), s.indexTTL)
		}); err != nil {
			return err
		}
	}

	// Create namespace workdir as a side effect.
	return os.MkdirAll(workdir, 0o700)
}

// keepPrevious carries over what a write that does not set it must not drop
// from the entry it overwrites: only a refless view marks the entry it settled
// on, so a refresh, an add or an install keeps the mark and its age, and the
// channel list.
func keepPrevious(s *store, ns domain.Namespace, file ManifestFile) (ManifestFile, time.Time) {
	var defaultAt time.Time
	if file.Default {
		defaultAt = s.clock()
	}
	prev, _, err := readCachedMeta(s, ns)
	if err != nil {
		return file, defaultAt
	}
	if !file.Default && prev.Default {
		file.Default, defaultAt = true, prev.DefaultAt
	}
	if len(file.Channels) == 0 {
		file.Channels = prev.Channels
	}
	return file, defaultAt
}

func deleteArrow(s *store, ns domain.Namespace) error {
	mu := s.namespaceLock(string(ns))
	mu.Lock()
	defer mu.Unlock()

	for _, name := range cacheNames(ns) {
		if err := deleteCacheEntry(s, name); err != nil {
			return err
		}
	}
	return nil
}

func deleteCacheEntry(s *store, name string) error {
	metaPath := filepath.Join(s.vaultPath, name+metaSuffix)
	meta, err := readMeta(metaPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil // idempotent
	}
	if err != nil {
		return err
	}

	if err := os.Remove(filepath.Join(s.vaultPath, name+filepath.Ext(meta.Filename))); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("vault delete: remove manifest: %w", err)
	}
	if err := os.Remove(metaPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("vault delete: remove meta: %w", err)
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
		if e.IsDir() || !strings.HasSuffix(e.Name(), metaSuffix) {
			continue
		}

		candidate, ok := s.cachedNamespace(e.Name())
		if !ok || candidate.BareNamespace() != bare {
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
	resolved, err := s.namespacePath(ns)
	if err != nil {
		return nil, "", err
	}
	return s.namespaceLock(ns.String()), resolved, nil
}

func getCollection(s *store, ns domain.Namespace) (*CollectionVaultEntry, string, error) {
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

	var onDisk quiverOnDisk
	if err := json.Unmarshal(data, &onDisk); err != nil {
		return nil, "", err
	}

	entry := &CollectionVaultEntry{
		Collection: onDisk.Collection,
		Metadata:   VaultMetadata{CachedAt: onDisk.CachedAt},
	}

	if s.clock().Sub(onDisk.CachedAt) > s.ttl {
		return entry, path, ErrStale
	}
	return entry, path, nil
}

func putCollection(
	s *store,
	ns domain.Namespace,
	coll *domain.Collection,
) (string, error) {
	mu, dir, err := acquireNamespace(s, ns)
	if err != nil {
		return "", err
	}
	mu.Lock()
	defer mu.Unlock()

	path := filepath.Join(dir, quiverFilename)

	onDisk := quiverOnDisk{
		Collection: coll,
		CachedAt:   s.clock(),
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

func listCachedQuivers(s *store) ([]domain.Namespace, error) {
	// Checked explicitly because os.ReadDir does not agree across platforms on
	// what a regular file is: Linux reports ENOTDIR, while Windows opens it and
	// returns no entries, so a corrupted namespaces path would read as "no
	// collections cached" there instead of failing.
	info, err := os.Stat(s.namespacesPath)
	if errors.Is(err, os.ErrNotExist) {
		return []domain.Namespace{}, nil
	}
	if err != nil {
		return []domain.Namespace{}, fmt.Errorf("vault list quivers: %w", err)
	}
	if !info.IsDir() {
		return []domain.Namespace{}, fmt.Errorf(
			"vault list quivers: %s is not a directory", s.namespacesPath,
		)
	}

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
		return []domain.Namespace{collectionNamespace(filepath.ToSlash(relPath), quiverPath)}, nil
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
		if e.Name() == "workdir" {
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

func deleteCollection(s *store, ns domain.Namespace) error {
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

// collectionNamespace names the collection cached at rel: decoded from the
// directory, or read from the file when the directory name was capped.
func collectionNamespace(
	rel string,
	quiverPath string,
) domain.Namespace {
	if !isHashed(rel) {
		return decodeNSDir(rel)
	}
	data, err := os.ReadFile(quiverPath) // #nosec G304 -- path comes from a walk under namespacesPath
	if err != nil {
		return decodeNSDir(rel)
	}
	var onDisk quiverOnDisk
	if json.Unmarshal(data, &onDisk) != nil || onDisk.Collection == nil || onDisk.Collection.Namespace == "" {
		return decodeNSDir(rel)
	}
	return onDisk.Collection.Namespace
}
