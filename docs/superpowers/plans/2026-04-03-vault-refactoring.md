# Vault Engine Refactoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the Vault engine interface and implementation to accept `context.Context`, return new envelope types (`VaultEntry`, `QuiverVaultEntry`), support indirect dependency tracking on `PutArrow`, and implement coexistence-safe directory deletion.

**Architecture:** The refactoring changes only types and signatures; core logic (locking, atomic writes, TTL validation, path sanitisation) remains unchanged. Return types shift from bare manifests to envelope types that include metadata and, for arrows, indirect dependencies. The `New` constructor becomes explicit about path and TTL configuration instead of hardcoding defaults.

**Tech Stack:** Go 1.21+, testify (assert/require), stdlib (sync, time, encoding/json, os, path/filepath)

---

## File Structure

**Files to modify:**
- `internal/engine/vault/vault_entry.go` — Add `VaultEntry`, `QuiverVaultEntry`, `VaultMetadata` exported types; keep private `vaultEntry[T]` for on-disk envelope
- `internal/engine/vault/vault.go` — Update interface method signatures (add `context.Context`, change return types)
- `internal/engine/vault/store.go` — Update method signatures, constructor, and internal field names
- `internal/engine/vault/manifest.go` — Specialize `getManifest` for Arrow/Quiver, update `putManifest` for `indirectDeps`, implement coexistence-safe `deleteManifest`
- `internal/engine/vault/vault_test.go` — Update all 17 test functions to use new signatures and access `.Manifest` on envelope types
- `internal/engine/vault/manifest_test.go` — Update manifest helper tests and add 3 new tests for `indirectDeps` and directory coexistence

**No changes needed:**
- `errors.go` (sentinel errors are correct)
- `manifest_bench_test.go` (benchmarks may need minor signature updates but no logic change)
- `mocks/fixtures.go` (helpers are still valid)

---

## Task 1: Create Exported Envelope Types in vault_entry.go

**Files:**
- Modify: `internal/engine/vault/vault_entry.go`
- Test: `internal/engine/vault/vault_test.go` (validation in later tasks)

**Why:** The app layer needs to access `IndirectDependencies` on arrow entries and metadata on quiver entries. The on-disk format must include these fields in the JSON envelope.

- [ ] **Step 1: Replace vault_entry.go with new exported types**

Open `internal/engine/vault/vault_entry.go` and replace with:

```go
package vault

import (
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

// VaultEntry is the value returned by GetArrow.
// IndirectDependencies is nil before the arrow has been installed;
// it is populated by PutArrow after DepTree resolves the full graph.
type VaultEntry struct {
	Manifest             *domain.ArrowManifest `json:"manifest"`
	Metadata             VaultMetadata         `json:"metadata"`
	IndirectDependencies []domain.Namespace    `json:"indirect_dependencies,omitempty"`
}

// QuiverVaultEntry is the value returned by GetQuiver.
type QuiverVaultEntry struct {
	Manifest *domain.QuiverManifest `json:"manifest"`
	Metadata VaultMetadata          `json:"metadata"`
}

// VaultMetadata records when and how a manifest was cached.
type VaultMetadata struct {
	CachedAt time.Time `json:"cached_at"`
	OS       string    `json:"os"`
}

// vaultEntry[T] is the internal on-disk envelope for any manifest type.
// This is used only by manifest helpers; it's not exported.
type vaultEntry[T any] struct {
	CachedAt time.Time `json:"cached_at"`
	OS       string    `json:"os"`
	Manifest T         `json:"manifest"`
}
```

- [ ] **Step 2: Verify file compiles**

Run: `go build ./internal/engine/vault/...`
Expected: Success (no errors about undefined types)

---

## Task 2: Update Vault Interface in vault.go

**Files:**
- Modify: `internal/engine/vault/vault.go:10-36`
- Test: `internal/engine/vault/vault_test.go` (tested in later tasks)

**Why:** All interface methods must accept `context.Context` as first parameter and return the new envelope types, not bare manifests.

- [ ] **Step 1: Update vault.go interface**

Open `internal/engine/vault/vault.go` and replace the `Vault` interface (lines 10-36) with:

```go
type Vault interface {
	// GetArrow returns the cached entry for the given namespace.
	// Returns ErrNotCached if no entry exists.
	// Returns ErrStale if TTL expired — entry and path are still returned.
	GetArrow(
		ctx context.Context,
		ns domain.Namespace,
	) (*VaultEntry, string, error)

	// GetQuiver returns the cached entry for the given namespace.
	// Same error semantics as GetArrow.
	GetQuiver(
		ctx context.Context,
		ns domain.Namespace,
	) (*QuiverVaultEntry, string, error)

	// PutArrow persists the manifest for the given namespace and returns the home directory path.
	// indirectDeps may be nil (pre-install) or populated (post-install, after DepTree runs).
	PutArrow(
		ctx context.Context,
		ns domain.Namespace,
		manifest *domain.ArrowManifest,
		indirectDeps []domain.Namespace,
	) (string, error)

	// PutQuiver persists the manifest for the given namespace and returns the home directory path.
	PutQuiver(
		ctx context.Context,
		ns domain.Namespace,
		manifest *domain.QuiverManifest,
	) (string, error)

	// DeleteArrow removes arrow.json. If quiver.json is absent too, removes the whole home directory.
	// Idempotent — returns nil if the entry does not exist.
	DeleteArrow(
		ctx context.Context,
		ns domain.Namespace,
	) error

	// DeleteQuiver removes quiver.json. If arrow.json is absent too, removes the whole home directory.
	// Idempotent — returns nil if the entry does not exist.
	DeleteQuiver(
		ctx context.Context,
		ns domain.Namespace,
	) error
}
```

Also add the import at the top if missing:

```go
import "context"
```

- [ ] **Step 2: Verify interface update compiles**

Run: `go build ./internal/engine/vault/...`
Expected: Fail with "store does not implement Vault" (store methods have wrong signatures)

---

## Task 3: Update store Struct and Constructor in store.go

**Files:**
- Modify: `internal/engine/vault/store.go:1-32`

**Why:** The `New` constructor must accept `basePath` and `ttl` as explicit parameters instead of hardcoding. The store struct itself remains unchanged.

- [ ] **Step 1: Update New constructor signature**

Open `internal/engine/vault/store.go` and replace the `New` function (lines 23-32) with:

```go
func New(
	basePath string,
	ttl time.Duration,
	osVersion string,
) Vault {
	if basePath == "" {
		basePath = metadata.GetQuiverHome()
	}
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	return &store{
		basePath:  basePath,
		ttl:       ttl,
		osVersion: osVersion,
		locks:     make(map[string]*sync.Mutex),
	}
}
```

Remove the `const vaultTTL` line (13) — it's no longer needed.

- [ ] **Step 2: Verify constructor compiles**

Run: `go build ./internal/engine/vault/...`
Expected: Still fails (store methods still have wrong signatures)

---

## Task 4: Update store Method Signatures in store.go

**Files:**
- Modify: `internal/engine/vault/store.go:64-118`

**Why:** All 6 interface methods must accept `context.Context` as the first parameter. The `ctx` is not used internally (all I/O is synchronous) but satisfies the interface and enables future cancellation support.

- [ ] **Step 1: Update GetArrow method**

Replace `func (s *store) GetArrow(...)` (lines 64-71) with:

```go
func (s *store) GetArrow(
	ctx context.Context,
	namespace domain.Namespace,
) (*VaultEntry, string, error) {
	if err := namespace.Validate(); err != nil {
		return nil, "", ErrInvalidNamespace
	}
	return getArrow(s, namespace)
}
```

Note: We're calling `getArrow` (a specialized helper) instead of the generic `getManifest`. We'll define this helper in the next task.

- [ ] **Step 2: Update GetQuiver method**

Replace `func (s *store) GetQuiver(...)` (lines 73-80) with:

```go
func (s *store) GetQuiver(
	ctx context.Context,
	namespace domain.Namespace,
) (*QuiverVaultEntry, string, error) {
	if err := namespace.Validate(); err != nil {
		return nil, "", ErrInvalidNamespace
	}
	return getQuiver(s, namespace)
}
```

- [ ] **Step 3: Update PutArrow method**

Replace `func (s *store) PutArrow(...)` (lines 82-90) with:

```go
func (s *store) PutArrow(
	ctx context.Context,
	namespace domain.Namespace,
	manifest *domain.ArrowManifest,
	indirectDeps []domain.Namespace,
) (string, error) {
	if err := namespace.Validate(); err != nil {
		return "", ErrInvalidNamespace
	}
	return putArrow(s, namespace, manifest, indirectDeps)
}
```

- [ ] **Step 4: Update PutQuiver method**

Replace `func (s *store) PutQuiver(...)` (lines 92-100) with:

```go
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
```

- [ ] **Step 5: Update DeleteArrow method**

Replace `func (s *store) DeleteArrow(...)` (lines 102-109) with:

```go
func (s *store) DeleteArrow(
	ctx context.Context,
	namespace domain.Namespace,
) error {
	if err := namespace.Validate(); err != nil {
		return ErrInvalidNamespace
	}
	return deleteArrow(s, namespace)
}
```

- [ ] **Step 6: Update DeleteQuiver method**

Replace `func (s *store) DeleteQuiver(...)` (lines 111-118) with:

```go
func (s *store) DeleteQuiver(
	ctx context.Context,
	namespace domain.Namespace,
) error {
	if err := namespace.Validate(); err != nil {
		return ErrInvalidNamespace
	}
	return deleteQuiver(s, namespace)
}
```

Add context import at the top:

```go
import "context"
```

- [ ] **Step 7: Verify signatures compile**

Run: `go build ./internal/engine/vault/...`
Expected: Still fails (manifest helpers don't exist yet)

---

## Task 5: Specialize Manifest Helpers in manifest.go — Arrow

**Files:**
- Modify: `internal/engine/vault/manifest.go`

**Why:** The generic `getManifest[T]` is replaced with specialized `getArrow` and `getQuiver` functions that return the correct envelope types. This makes the type conversions explicit and simpler.

- [ ] **Step 1: Add getArrow helper**

In `manifest.go`, after the imports and before the existing generic `getManifest`, add:

```go
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
```

- [ ] **Step 2: Verify getArrow compiles**

Run: `go build ./internal/engine/vault/...`
Expected: Still fails (getQuiver, putArrow, putQuiver, deleteArrow, deleteQuiver not defined)

---

## Task 6: Add getQuiver and Specialized Manifest Helpers in manifest.go

**Files:**
- Modify: `internal/engine/vault/manifest.go`

**Why:** Complete the specialized helpers for quiver retrieval and both arrow/quiver mutations.

- [ ] **Step 1: Add getQuiver helper**

In `manifest.go`, after `getArrow`, add:

```go
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
```

- [ ] **Step 2: Add putArrow helper**

After `getQuiver`, add:

```go
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
```

- [ ] **Step 3: Add putQuiver helper**

After `putArrow`, add:

```go
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
```

- [ ] **Step 4: Verify put helpers compile**

Run: `go build ./internal/engine/vault/...`
Expected: Still fails (deleteArrow, deleteQuiver not defined)

---

## Task 7: Add Delete Helpers with Coexistence-Safe Logic in manifest.go

**Files:**
- Modify: `internal/engine/vault/manifest.go`

**Why:** `deleteArrow` and `deleteQuiver` must check if the sibling file exists before removing the directory. This ensures that if both arrow.json and quiver.json coexist, deleting one doesn't remove the other's directory.

- [ ] **Step 1: Add deleteArrow helper**

After `putQuiver`, add:

```go
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
```

- [ ] **Step 2: Add deleteQuiver helper**

After `deleteArrow`, add:

```go
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
```

- [ ] **Step 3: Keep or remove old generic helpers**

The old `getManifest[T]`, `putManifest[T]`, and `deleteManifest` generics are no longer called by store methods. You have two options:

**Option A (Clean):** Delete them entirely. They are not part of the public API.

**Option B (Cautious):** Keep them but mark them as deprecated. This helps if there are other callers outside the vault package (check with grep).

For now, assume **Option A**: Remove the old generic functions. We'll verify no other code calls them in the next task.

- [ ] **Step 4: Verify all helpers compile**

Run: `go build ./internal/engine/vault/...`
Expected: Success! All interface methods now have valid implementations.

---

## Task 8: Update vault_test.go for New Signatures

**Files:**
- Modify: `internal/engine/vault/vault_test.go`
- Test: Tests will pass after the update

**Why:** All tests must be updated to:
1. Pass `context.Context` (use `context.Background()`)
2. Access `.Manifest.Name` instead of `.Name` on envelope types
3. Update the helper function to use the new constructor

- [ ] **Step 1: Update newTestVault helper**

Replace `func newTestVault(...)` (lines 14-23) with:

```go
func newTestVault(
	t *testing.T,
) Vault {
	return New(t.TempDir(), time.Hour, "darwin/arm64")
}
```

- [ ] **Step 2: Update TestGetArrow_NotCached**

Replace `func TestGetArrow_NotCached(...)` (lines 27-33) with:

```go
func TestGetArrow_NotCached(t *testing.T) {
	v := newTestVault(t)

	_, _, err := v.GetArrow(context.Background(), mocks.Namespace())

	assert.ErrorIs(t, err, ErrNotCached)
}
```

Add import: `"context"`

- [ ] **Step 3: Update TestGetArrow_Fresh**

Replace `func TestGetArrow_Fresh(...)` (lines 35-48) with:

```go
func TestGetArrow_Fresh(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()

	_, err := v.PutArrow(context.Background(), ns, arrow, nil)
	require.NoError(t, err)

	got, path, err := v.GetArrow(context.Background(), ns)

	require.NoError(t, err)
	assert.Equal(t, arrow.Name, got.Manifest.Name)
	assert.NotEmpty(t, path)
}
```

- [ ] **Step 4: Update TestGetArrow_InvalidNamespace**

Replace with:

```go
func TestGetArrow_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	_, _, err := v.GetArrow(context.Background(), domain.Namespace(""))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}
```

- [ ] **Step 5: Update TestGetQuiver_NotCached**

Replace with:

```go
func TestGetQuiver_NotCached(t *testing.T) {
	v := newTestVault(t)

	_, _, err := v.GetQuiver(context.Background(), mocks.Namespace())

	assert.ErrorIs(t, err, ErrNotCached)
}
```

- [ ] **Step 6: Update TestGetQuiver_Fresh**

Replace with:

```go
func TestGetQuiver_Fresh(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	quiver := mocks.QuiverManifest()

	_, err := v.PutQuiver(context.Background(), ns, quiver)
	require.NoError(t, err)

	got, path, err := v.GetQuiver(context.Background(), ns)

	require.NoError(t, err)
	assert.Equal(t, quiver.Name, got.Manifest.Name)
	assert.NotEmpty(t, path)
}
```

- [ ] **Step 7: Update TestGetQuiver_InvalidNamespace**

Replace with:

```go
func TestGetQuiver_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	_, _, err := v.GetQuiver(context.Background(), domain.Namespace(""))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}
```

- [ ] **Step 8: Update TestPutArrow_CreatesFile**

Replace with:

```go
func TestPutArrow_CreatesFile(t *testing.T) {
	v := newTestVault(t)

	path, err := v.PutArrow(context.Background(), mocks.Namespace(), mocks.ArrowManifest(), nil)

	require.NoError(t, err)
	assert.NotEmpty(t, path)
}
```

- [ ] **Step 9: Update TestPutArrow_OverwritesExisting**

Replace with:

```go
func TestPutArrow_OverwritesExisting(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutArrow(context.Background(), ns, &domain.ArrowManifest{Name: "first"}, nil)
	require.NoError(t, err)
	_, err = v.PutArrow(context.Background(), ns, &domain.ArrowManifest{Name: "second"}, nil)
	require.NoError(t, err)

	got, _, err := v.GetArrow(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, "second", got.Manifest.Name)
}
```

- [ ] **Step 10: Update TestPutArrow_InvalidNamespace**

Replace with:

```go
func TestPutArrow_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	_, err := v.PutArrow(context.Background(), domain.Namespace(""), mocks.ArrowManifest(), nil)

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}
```

- [ ] **Step 11: Update TestPutQuiver_CreatesFile**

Replace with:

```go
func TestPutQuiver_CreatesFile(t *testing.T) {
	v := newTestVault(t)

	path, err := v.PutQuiver(context.Background(), mocks.Namespace(), mocks.QuiverManifest())

	require.NoError(t, err)
	assert.NotEmpty(t, path)
}
```

- [ ] **Step 12: Update TestPutQuiver_InvalidNamespace**

Replace with:

```go
func TestPutQuiver_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	_, err := v.PutQuiver(context.Background(), domain.Namespace(""), mocks.QuiverManifest())

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}
```

- [ ] **Step 13: Update TestDeleteArrow_RemovesFile**

Replace with:

```go
func TestDeleteArrow_RemovesFile(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutArrow(context.Background(), ns, mocks.ArrowManifest(), nil)
	require.NoError(t, err)

	require.NoError(t, v.DeleteArrow(context.Background(), ns))

	_, _, err = v.GetArrow(context.Background(), ns)
	assert.ErrorIs(t, err, ErrNotCached)
}
```

- [ ] **Step 14: Update TestDeleteArrow_Idempotent**

Replace with:

```go
func TestDeleteArrow_Idempotent(t *testing.T) {
	v := newTestVault(t)

	assert.NoError(t, v.DeleteArrow(context.Background(), mocks.Namespace()))
}
```

- [ ] **Step 15: Update TestDeleteArrow_InvalidNamespace**

Replace with:

```go
func TestDeleteArrow_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	err := v.DeleteArrow(context.Background(), domain.Namespace(""))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}
```

- [ ] **Step 16: Update TestDeleteQuiver_RemovesFile**

Replace with:

```go
func TestDeleteQuiver_RemovesFile(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutQuiver(context.Background(), ns, mocks.QuiverManifest())
	require.NoError(t, err)

	require.NoError(t, v.DeleteQuiver(context.Background(), ns))

	_, _, err = v.GetQuiver(context.Background(), ns)
	assert.ErrorIs(t, err, ErrNotCached)
}
```

- [ ] **Step 17: Update TestDeleteQuiver_Idempotent**

Replace with:

```go
func TestDeleteQuiver_Idempotent(t *testing.T) {
	v := newTestVault(t)

	assert.NoError(t, v.DeleteQuiver(context.Background(), mocks.Namespace()))
}
```

- [ ] **Step 18: Update TestDeleteQuiver_InvalidNamespace**

Replace with:

```go
func TestDeleteQuiver_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	err := v.DeleteQuiver(context.Background(), domain.Namespace(""))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}
```

- [ ] **Step 19: Update TestArrowAndQuiverCoexist**

Replace with:

```go
func TestArrowAndQuiverCoexist(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()
	quiver := mocks.QuiverManifest()

	_, err := v.PutArrow(context.Background(), ns, arrow, nil)
	require.NoError(t, err)
	_, err = v.PutQuiver(context.Background(), ns, quiver)
	require.NoError(t, err)

	gotArrow, _, err := v.GetArrow(context.Background(), ns)
	require.NoError(t, err)
	gotQuiver, _, err := v.GetQuiver(context.Background(), ns)
	require.NoError(t, err)

	assert.Equal(t, arrow.Name, gotArrow.Manifest.Name)
	assert.Equal(t, quiver.Name, gotQuiver.Manifest.Name)
}
```

- [ ] **Step 20: Update TestPutArrow_Concurrent**

Replace with:

```go
func TestPutArrow_Concurrent(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v.PutArrow(context.Background(), ns, arrow, nil)
		}()
	}
	wg.Wait()

	_, _, err := v.GetArrow(context.Background(), ns)
	assert.NoError(t, err)
}
```

- [ ] **Step 21: Update TestConcurrentGetPutArrow**

Replace with:

```go
func TestConcurrentGetPutArrow(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()

	_, err := v.PutArrow(context.Background(), ns, arrow, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			v.GetArrow(context.Background(), ns)
		}()
		go func() {
			defer wg.Done()
			v.PutArrow(context.Background(), ns, arrow, nil)
		}()
	}
	wg.Wait()
}
```

- [ ] **Step 22: Update TestConcurrentDeleteGetArrow**

Replace with:

```go
func TestConcurrentDeleteGetArrow(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()

	_, err := v.PutArrow(context.Background(), ns, arrow, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			v.DeleteArrow(context.Background(), ns)
		}()
		go func() {
			defer wg.Done()
			v.GetArrow(context.Background(), ns)
		}()
	}
	wg.Wait()
}
```

- [ ] **Step 23: Add three new tests for IndirectDeps and Directory Coexistence**

At the end of `vault_test.go`, add:

```go
// IndirectDependencies

func TestPutArrow_PersistsIndirectDeps(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()
	indirectDeps := []domain.Namespace{
		domain.Namespace("github.com/foo/bar"),
		domain.Namespace("github.com/baz/qux"),
	}

	_, err := v.PutArrow(context.Background(), ns, arrow, indirectDeps)
	require.NoError(t, err)

	got, _, err := v.GetArrow(context.Background(), ns)
	require.NoError(t, err)

	assert.Equal(t, indirectDeps, got.IndirectDependencies)
}

// Coexistence: directory deletion

func TestDeleteArrow_RemovesDirectoryWhenEmpty(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutArrow(context.Background(), ns, mocks.ArrowManifest(), nil)
	require.NoError(t, err)

	// Get the directory path
	mu, dir, err := v.(*store).acquireNamespace(ns)
	require.NoError(t, err)
	mu.Lock()
	mu.Unlock()

	require.NoError(t, v.DeleteArrow(context.Background(), ns))

	// Directory should be gone
	_, err = os.Stat(dir)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestDeleteArrow_PreservesDirectoryWhenQuiverExists(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutArrow(context.Background(), ns, mocks.ArrowManifest(), nil)
	require.NoError(t, err)
	_, err = v.PutQuiver(context.Background(), ns, mocks.QuiverManifest())
	require.NoError(t, err)

	// Get the directory path
	mu, dir, err := v.(*store).acquireNamespace(ns)
	require.NoError(t, err)
	mu.Lock()
	mu.Unlock()

	require.NoError(t, v.DeleteArrow(context.Background(), ns))

	// Directory should still exist (quiver is there)
	_, err = os.Stat(dir)
	assert.NoError(t, err)

	// But arrow.json should be gone
	_, _, err = v.GetArrow(context.Background(), ns)
	assert.ErrorIs(t, err, ErrNotCached)

	// And quiver.json should still be there
	_, _, err = v.GetQuiver(context.Background(), ns)
	assert.NoError(t, err)
}
```

Add imports if missing:

```go
import (
	"context"
	"errors"
	"os"
)
```

- [ ] **Step 24: Run all tests**

Run: `go test ./internal/engine/vault/... -v`
Expected: All tests pass (16 updated + 3 new = 19 new tests in vault_test.go)

---

## Task 9: Update manifest_test.go for New Signatures

**Files:**
- Modify: `internal/engine/vault/manifest_test.go`

**Why:** The manifest helper tests currently use the generic `getManifest[T]` and `putManifest[T]` which no longer exist. Tests must be rewritten to call the specialized `getArrow`, `putArrow`, `getQuiver`, `putQuiver`, `deleteArrow`, `deleteQuiver` functions or removed entirely if they test internal logic no longer exposed.

**Decision:** Since the old generic helpers are now internal implementation details replaced by specialized functions, and the interface tests in `vault_test.go` already cover the behavior, **remove the manifest_test.go file entirely**. The tests for `getManifest`, `putManifest`, and `deleteManifest` in that file are testing private functions that no longer exist in their old form.

- [ ] **Step 1: Delete manifest_test.go**

Run: `rm internal/engine/vault/manifest_test.go`

(Alternatively, if you want to keep some coverage of the helpers, you could rewrite the tests to use the new `getArrow`/`putArrow` functions, but given the vault_test.go already covers all scenarios, deletion is simpler and avoids duplication.)

- [ ] **Step 2: Update writeStaleEntry usage if needed**

If `writeStaleEntry` is used only in `manifest_test.go`, it can be removed. Check if it's used in `vault_test.go`:

Run: `grep -n "writeStaleEntry" internal/engine/vault/*.go`
Expected: No matches (it's only in manifest_test.go)

If there are no matches, no further action needed.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/engine/vault/... -v`
Expected: All vault_test.go tests pass; manifest_test.go no longer exists.

---

## Task 10: Final Verification

**Files:**
- All files in `internal/engine/vault/`

**Why:** Ensure the entire package builds, all tests pass, and there are no remaining compilation errors.

- [ ] **Step 1: Build the package**

Run: `go build ./internal/engine/vault/...`
Expected: Success (no errors)

- [ ] **Step 2: Run all tests**

Run: `go test ./internal/engine/vault/... -v`
Expected: All tests pass

- [ ] **Step 3: Check for unused code**

The old `getManifest[T]`, `putManifest[T]`, and `deleteManifest` generic functions should be deleted from manifest.go. If they're still there, remove them:

Run: `grep -n "func getManifest\|func putManifest\|func deleteManifest" internal/engine/vault/manifest.go`
Expected: No matches (all old generics deleted)

- [ ] **Step 4: Verify on-disk format is correct**

The JSON written to disk should now have the new structure. You can verify by examining a test's temp directory after PutArrow, e.g., by adding a debug print or by reading the file created in step 1 of Task 8.

Expected format for arrow.json:
```json
{
  "manifest": { "name": "...", ... },
  "cached_at": "2026-04-03T...",
  "os": "darwin/arm64",
  "indirect_dependencies": ["github.com/foo/bar"]
}
```

Expected format for quiver.json:
```json
{
  "manifest": { "name": "...", ... },
  "cached_at": "2026-04-03T...",
  "os": "darwin/arm64"
}
```

- [ ] **Step 5: Run a quick smoke test by hand (optional)**

If you want extra confidence, manually create a vault and put/get an arrow:

```bash
cd internal/engine/vault
go run -run TestPutArrow_PersistsIndirectDeps
```

Expected: Test passes and prints no errors.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor(vault): add context.Context, new envelope types, indirect deps tracking, coexistence-safe delete"
```

---

## Spec Coverage Checklist

- [x] Add `context.Context` to all 6 interface methods (**Task 2**)
- [x] Change `GetArrow` return to `*VaultEntry` (**Task 2, 5**)
- [x] Change `GetQuiver` return to `*QuiverVaultEntry` (**Task 2, 6**)
- [x] Add `indirectDeps []domain.Namespace` to `PutArrow` (**Task 2, 6**)
- [x] Create exported types: `VaultEntry`, `QuiverVaultEntry`, `VaultMetadata` (**Task 1**)
- [x] Update `New` constructor with `basePath` and `ttl` parameters (**Task 3**)
- [x] Implement coexistence-safe delete logic (**Task 7**)
- [x] Update on-disk JSON format (**Task 1, 5, 6**)
- [x] Update all tests (**Task 8, 9**)
- [x] Add new tests for `indirectDeps` and directory coexistence (**Task 8, Step 23**)

---

## Notes for the Engineer

- **Context**: The `ctx` parameter is not used internally (all I/O is synchronous) but must be accepted for interface compliance and future cancellation support.
- **Locking**: Locking rules remain unchanged. `getArrow`/`getQuiver` acquire the lock, `putArrow`/`putQuiver`/`deleteArrow`/`deleteQuiver` acquire the lock.
- **Atomic writes**: The atomic write pattern (write to temp, rename) is unchanged in the new `putArrow` and `putQuiver` helpers.
- **Old generics**: The old `getManifest[T]`, `putManifest[T]`, and `deleteManifest` must be deleted after the new helpers are in place. They are not part of the public API.
- **Directory removal safety**: After deleting arrow.json or quiver.json, the code checks if the sibling file exists. If not, the entire directory is removed. This is safe because the per-namespace lock ensures no concurrent operations on the same namespace during delete.

