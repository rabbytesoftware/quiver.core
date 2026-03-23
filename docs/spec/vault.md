# Quiver — Vault

## Overview

Vault is the infrastructure module responsible for **persisting and caching assembled domain objects** on disk. It stores fully resolved `ArrowManifest` and `QuiverManifest` objects as JSON, wrapped in an envelope that carries metadata (fetch timestamp, source URL, eviction TTL).

Vault replaces the disk persistence concern that was previously bundled inside Manifold. By storing the **assembled domain object** rather than raw YAML, reads from Vault skip the Translator and Assembler pipeline entirely — making cached reads fast and eliminating re-parsing.

The app layer composes Vault and Manifold: check Vault first, call Manifold on cache miss, then persist via Vault.

---

## 1. Module Name

`vault` — the manifest persistence and caching module.

The package lives at `internal/infrastructure/vault`.

---

## 2. Interface Contract

The app layer depends on a single interface:

```go
// VaultPort is the interface the app layer depends on.
// It is defined in the app layer — vault implements it.
type VaultPort interface {
    // GetArrow retrieves a cached ArrowManifest for the given namespace.
    // Returns ErrNotCached if no entry exists for this namespace.
    // The caller is responsible for checking IsStale() on the returned entry.
    GetArrow(ctx context.Context, ns Namespace) (*VaultEntry, error)

    // PutArrow persists an ArrowManifest with its metadata and optional indirect dependencies.
    // Overwrites any existing entry for this namespace.
    // indirectDeps may be nil (pre-install) or populated (post-install, after DepTree resolution).
    PutArrow(ctx context.Context, ns Namespace, manifest *ArrowManifest, meta VaultMetadata, indirectDeps []Namespace) error

    // DeleteArrow removes a cached ArrowManifest entry.
    // Returns nil if the entry does not exist (idempotent).
    DeleteArrow(ctx context.Context, ns Namespace) error

    // GetQuiver retrieves a cached QuiverManifest for the given namespace.
    // Returns ErrNotCached if no entry exists.
    GetQuiver(ctx context.Context, ns Namespace) (*QuiverVaultEntry, error)

    // PutQuiver persists a QuiverManifest with its metadata.
    // Overwrites any existing entry for this namespace.
    PutQuiver(ctx context.Context, ns Namespace, manifest *QuiverManifest, meta VaultMetadata) error

    // DeleteQuiver removes a cached QuiverManifest entry.
    // Returns nil if the entry does not exist (idempotent).
    DeleteQuiver(ctx context.Context, ns Namespace) error
}
```

This is the **only** interface the app layer imports. No file paths, no JSON encoding details — just domain objects in and out.

---

## 3. Storage Format

### 3.1 JSON Envelope

Each entry is a single JSON file containing the manifest and its metadata in an envelope:

```json
{
  "metadata": {
    "fetched_at": "2026-03-22T14:30:00Z",
    "source": "https://github.com/valve/steamcmd",
    "file": "arrow.yaml",
    "os": "linux",
    "eviction_ttl": "48h"
  },
  "manifest": {
    "name": "SteamCMD",
    "description": "Valve's command-line tool for installing and updating dedicated servers",
    "version": "0.0.1",
    "license": "Proprietary",
    "dependencies": ["github.com/valve/steam-runtime"],
    "lifecycle": {
      "install": [...],
      "uninstall": [...]
    }
  },
  "indirect_dependencies": ["github.com/valve/glibc-compat"]
}
```

The `manifest` field contains the **fully assembled domain object** — OS overrides already resolved, timeouts parsed, validation passed. This is the output of Manifold's Assembler, serialized as JSON.

### 3.2 Why JSON, Not YAML

The Vault stores assembled domain objects, not raw manifest files. JSON is chosen because:

- Domain structs serialize cleanly to JSON via Go's `encoding/json`.
- No ambiguity around YAML anchors, aliases, or multiline strings.
- Faster to parse than YAML for structured data.
- Clear separation: YAML is the author-facing format (manifest files in git repos). JSON is the internal cache format.

---

## 4. Data Model

### 4.1 VaultEntry (Arrow)

```go
type VaultEntry struct {
    Manifest             *ArrowManifest `json:"manifest"`
    Metadata             VaultMetadata  `json:"metadata"`
    IndirectDependencies []Namespace    `json:"indirect_dependencies,omitempty"`
}
```

`IndirectDependencies` holds the **transitive** dependencies resolved by DepTree during the install use case. These are all dependencies in the DepTree result that are not the root arrow itself and not in `Manifest.Dependencies` (which are the direct dependencies). This field is `nil` before the arrow has been installed — it is populated after DepTree runs successfully.

### 4.2 QuiverVaultEntry

```go
type QuiverVaultEntry struct {
    Manifest *QuiverManifest `json:"manifest"`
    Metadata VaultMetadata   `json:"metadata"`
}
```

### 4.3 VaultMetadata

```go
type VaultMetadata struct {
    FetchedAt   time.Time `json:"fetched_at"`
    Source      string    `json:"source"`        // git clone URL used to fetch
    File        string    `json:"file"`          // original filename in repo (arrow.yaml, cs2.yaml, etc.)
    OS          string    `json:"os"`            // OS the manifest was resolved for (informational)
    EvictionTTL string    `json:"eviction_ttl"`  // e.g. "48h" — stored so Vault can check staleness
}
```

### 4.4 Staleness Check

```go
func (e *VaultEntry) IsStale() bool {
    ttl, err := time.ParseDuration(e.Metadata.EvictionTTL)
    if err != nil {
        return true // unparseable TTL = always stale
    }
    return time.Since(e.Metadata.FetchedAt) > ttl
}
```

`IsStale()` is a method on the entry, not on Vault. The caller (app layer) decides what to do with a stale entry — Vault just reports the state.

The same method exists on `QuiverVaultEntry`.

### 4.5 Indirect Dependencies Lifecycle

The `IndirectDependencies` field on `VaultEntry` follows a specific lifecycle:

| Event | IndirectDependencies state |
|-------|---------------------------|
| Manifest cached (add / Manifold fetch) | `nil` — DepTree has not run yet |
| Install completes (DepTree resolved) | Populated — all transitive deps computed |
| Re-install / update | Re-populated — DepTree runs again, deps may have changed |
| Eviction (TTL expired, re-fetch) | Preserved — the app layer re-populates after re-fetch if the arrow was previously installed |

**Population logic (app layer):**

After DepTree returns `[]Namespace` in topological order, the app layer computes indirect dependencies for the root arrow:

```go
// deptreeResult is []Namespace in topological order (deps first, root last)
// root is the arrow being installed
// directDeps is root's Manifest.Dependencies

indirect := []Namespace{}
directSet := toSet(directDeps)
for _, ns := range deptreeResult {
    if ns == root {
        continue
    }
    if !directSet[ns] {
        indirect = append(indirect, ns)
    }
}
// Update Vault entry with indirect
```

The app layer also updates the Vault entries of each **dependency** with their own `indirect_dependencies` as a side effect of the install — since DepTree has already resolved the full graph, the data is available at no extra cost.

---

## 5. Keying

Entries are keyed by **namespace only**. The OS is determined at install time and does not change for a given Quiver instance. The `OS` field in metadata is informational — it records which OS the manifest was resolved for but does not participate in cache lookup.

---

## 6. Disk Layout

### 6.1 Directory Structure

```
~/.quiver/vault/arrows/github.com/valve/steamcmd/entry.json
~/.quiver/vault/arrows/github.com/char2cs/gaming.quiver/cs2/entry.json
~/.quiver/vault/quivers/github.com/char2cs/gaming.quiver/entry.json
```

The namespace segments map directly to directory segments — same convention as the previous Manifold layout, but under `~/.quiver/vault/` to avoid collision with Arrow working directories.

### 6.2 Path Mapping

| Type | Namespace | Vault Path |
|------|-----------|------------|
| Standalone Arrow | `github.com/valve/steamcmd` | `vault/arrows/github.com/valve/steamcmd/entry.json` |
| Quiver Arrow | `github.com/char2cs/gaming.quiver/cs2` | `vault/arrows/github.com/char2cs/gaming.quiver/cs2/entry.json` |
| Quiver | `github.com/char2cs/gaming.quiver` | `vault/quivers/github.com/char2cs/gaming.quiver/entry.json` |

### 6.3 Single File Per Entry

Each namespace gets one file: `entry.json`. No separate metadata file — the metadata is embedded in the envelope. This simplifies reads (one file open, one unmarshal) and writes (one atomic operation).

---

## 7. Operations

### 7.1 Get

1. Compute path: `basePath/{type}/{namespace}/entry.json`
2. Read file
3. If file does not exist → return `ErrNotCached`
4. JSON unmarshal into `VaultEntry` or `QuiverVaultEntry`
5. Return the entry (caller checks `IsStale()`)

### 7.2 Put

1. Compute path
2. Create directory tree if it does not exist (`MkdirAll`)
3. JSON marshal the entry (metadata + manifest)
4. Write to a temporary file in the same directory, then rename to `entry.json` (atomic write)
5. If rename is not supported (edge case), fall back to direct write

Atomic write prevents partial reads if a Get happens concurrently.

### 7.3 Delete

1. Compute path
2. Remove the namespace directory and its contents (`RemoveAll`)
3. If the directory does not exist, return nil (idempotent)

---

## 8. Error Types

```go
var (
    // ErrNotCached indicates no entry exists in the Vault for the given namespace.
    ErrNotCached = errors.New("vault: entry not found in cache")
)
```

All other errors (disk I/O failures, JSON marshal/unmarshal failures) propagate as wrapped errors with context.

---

## 9. Concurrency

### 9.1 Strategy

Per-namespace mutex using `sync.Map`:

```go
type Vault struct {
    basePath string
    locks    sync.Map // map[string]*sync.Mutex
}

func (v *Vault) lock(ns Namespace) *sync.Mutex {
    mu, _ := v.locks.LoadOrStore(ns.String(), &sync.Mutex{})
    return mu.(*sync.Mutex)
}
```

### 9.2 Locking Rules

| Operation | Lock Required |
|-----------|---------------|
| Get | No — reads are safe. A concurrent Put uses atomic rename, so Get either sees the old file or the new file, never a partial write. |
| Put | Yes — acquires per-namespace lock. Prevents two concurrent Puts from racing on the same entry. |
| Delete | Yes — acquires per-namespace lock. Prevents Delete from racing with Put. |

---

## 10. Configuration

Eviction TTLs are configured at the application level, not in the Vault module:

```yaml
config:
  vault:
    arrow_ttl: "48h"
    quiver_ttl: "12h"
```

The app layer reads these values from config and passes them as part of `VaultMetadata` when calling `PutArrow` or `PutQuiver`. Vault itself does not read configuration — it receives TTLs at write time and evaluates them at read time via `IsStale()`. This keeps Vault configuration-unaware and testable.

---

## 11. App Layer Composition

Vault is not called in isolation. The app layer composes Vault with Manifold in a **cache-first pattern**:

### 11.1 Manifest Resolution (Cache-First)

```go
func (svc *ArrowService) resolveManifest(ctx context.Context, ns Namespace, os string) (*ArrowManifest, error) {
    // 1. Check Vault
    entry, err := svc.vault.GetArrow(ctx, ns)
    if err == nil && !entry.IsStale() {
        return entry.Manifest, nil
    }

    // 2. Vault miss or stale — fetch from Manifold (git)
    manifest, fetchErr := svc.manifold.ResolveArrow(ctx, ns, os)
    if fetchErr != nil {
        // 3. Manifold failed — if stale entry exists, serve it with a warning
        if err == nil {
            // log.Warn("using stale cache for %s, fetch failed: %v", ns, fetchErr)
            return entry.Manifest, nil
        }
        return nil, fetchErr
    }

    // 4. Fresh manifest — cache in Vault
    _ = svc.vault.PutArrow(ctx, ns, manifest, VaultMetadata{
        FetchedAt:   time.Now(),
        Source:      deriveGitURL(ns),
        File:        deriveFilename(ns),
        OS:          os,
        EvictionTTL: svc.config.Vault.ArrowTTL,
    })

    return manifest, nil
}
```

This preserves the graceful degradation behavior from the original Manifold spec: if the remote fetch fails but a stale cached copy exists, the stale copy is returned with a logged warning.

### 11.2 DepTree Resolver Callback

The resolver callback passed to DepTree uses the same cache-first pattern. It resolves a manifest and extracts the dependency list:

```go
resolver := func(ctx context.Context, ns Namespace) ([]Namespace, error) {
    manifest, err := svc.resolveManifest(ctx, ns, os)
    if err != nil {
        return nil, err
    }
    return manifest.Dependencies, nil
}
```

### 11.3 Post-Install Vault Update

After DepTree resolves the full graph and installation completes, the app layer updates the Vault entry with `indirect_dependencies`:

```go
func (svc *ArrowService) updateIndirectDeps(ctx context.Context, ns Namespace, deptreeResult []Namespace) error {
    entry, err := svc.vault.GetArrow(ctx, ns)
    if err != nil {
        return err
    }

    directSet := make(map[string]bool, len(entry.Manifest.Dependencies))
    for _, d := range entry.Manifest.Dependencies {
        directSet[d.String()] = true
    }

    var indirect []Namespace
    for _, dep := range deptreeResult {
        if dep == ns {
            continue
        }
        if !directSet[dep.String()] {
            indirect = append(indirect, dep)
        }
    }

    return svc.vault.PutArrow(ctx, ns, entry.Manifest, entry.Metadata, indirect)
}
```

---

## 12. Constraints

- **No Asynx knowledge** — Vault is pure infrastructure. It does not emit events or commands.
- **No network I/O** — Vault only reads and writes to the local filesystem.
- **No manifest parsing** — Vault stores and retrieves pre-assembled domain objects. It does not understand YAML, schemas, or business rules.
- **No configuration awareness** — TTLs are received as data, not read from config.
- **App layer is the only caller** — no other layer imports `VaultPort`.
- **Idempotent deletes** — deleting a non-existent entry is not an error.

---

## 13. File Layout

```
internal/infrastructure/vault/
    vault.go    — Vault struct, New(), GetArrow, PutArrow, DeleteArrow, GetQuiver, PutQuiver, DeleteQuiver
    models.go   — VaultEntry, QuiverVaultEntry, VaultMetadata, IsStale()
    errors.go   — ErrNotCached
```

---

## 14. Open Questions

| # | Question | Default if unresolved |
|---|----------|-----------------------|
| 1 | Should Vault support a `Purge()` method to clear all cached entries? | Not in v0 — add if a "clear cache" user command is needed. |
| 2 | Should Vault validate the JSON against the current domain types on read (to handle schema drift after upgrades)? | No — if the JSON can't unmarshal into the current struct, the entry is treated as missing (`ErrNotCached`-equivalent). The app layer re-fetches via Manifold. |
| 3 | Should Vault support listing all cached namespaces? | Not in v0 — add if a cache inspection CLI is needed. |
| 4 | When a Vault entry is evicted and re-fetched (manifest updated via Manifold), should `indirect_dependencies` be preserved from the previous entry or cleared? | Preserved — the app layer copies `IndirectDependencies` from the old entry to the new one if the arrow was previously installed. DepTree only re-runs on explicit re-install. |
