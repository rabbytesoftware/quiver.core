# Quiver — Vault

## Overview

Vault is the infrastructure module responsible for **allocating namespace home directories and persisting assembled domain objects** on disk. It stores fully resolved `ArrowManifest` and `QuiverManifest` objects as JSON, wrapped in an envelope that carries metadata (fetch timestamp, source URL, eviction TTL).

When an Arrow or Quiver is added, the app layer passes the assembled manifest to Vault. Vault allocates a home directory under `~/.quiver/namespaces/`, writes the manifest as JSON, and returns the path. This home directory is the Arrow's workspace for the rest of its lifecycle — install artifacts, downloaded files, and runtime state all live here alongside the manifest.

The app layer composes Vault and Manifold: call Manifold to fetch and assemble the manifest, then persist via Vault. On subsequent reads, check Vault first — call Manifold only on cache miss or stale entry.

---

## 1. Module Name

`vault` — the namespace home directory and manifest persistence module.

The package lives at `internal/infrastructure/vault`.

---

## 2. Interface Contract

The app layer depends on a single interface:

```go
// Vault is the interface the app layer depends on.
// Defined in the vault package.
type Vault interface {
    // PutArrow allocates a home directory for the namespace (if it doesn't exist),
    // persists the ArrowManifest as arrow.json, and returns the home directory path.
    // Overwrites any existing arrow.json for this namespace.
    // indirectDeps may be nil (pre-install) or populated (post-install, after DepTree resolution).
    PutArrow(ctx context.Context, ns Namespace, manifest *ArrowManifest, indirectDeps []Namespace) (string, error)

    // GetArrow retrieves a cached ArrowManifest for the given namespace.
    // Returns the entry, the home directory path, and an error.
    //
    // Error semantics:
    //   - nil:          fresh cache hit (TTL not expired)
    //   - ErrStale:     entry exists but TTL expired — entry and path ARE still returned
    //   - ErrNotCached: no entry exists for this namespace
    GetArrow(ctx context.Context, ns Namespace) (*VaultEntry, string, error)

    // DeleteArrow removes the arrow.json file.
    // If no quiver.json exists in the same directory, removes the entire home directory.
    // Returns nil if the entry does not exist (idempotent).
    DeleteArrow(ctx context.Context, ns Namespace) error

    // PutQuiver allocates a home directory for the namespace (if it doesn't exist),
    // persists the QuiverManifest as quiver.json, and returns the home directory path.
    // Overwrites any existing quiver.json for this namespace.
    PutQuiver(ctx context.Context, ns Namespace, manifest *QuiverManifest) (string, error)

    // GetQuiver retrieves a cached QuiverManifest for the given namespace.
    // Returns the entry, the home directory path, and an error.
    //
    // Error semantics are identical to GetArrow.
    GetQuiver(ctx context.Context, ns Namespace) (*QuiverVaultEntry, string, error)

    // DeleteQuiver removes the quiver.json file.
    // If no arrow.json exists in the same directory, removes the entire home directory.
    // Returns nil if the entry does not exist (idempotent).
    DeleteQuiver(ctx context.Context, ns Namespace) error
}
```

This is the **only** interface the app layer imports. Vault owns all path resolution — callers never compute home directory paths.

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
- Clear separation: YAML is the author-facing format (manifest files in git repos). JSON is the internal persistence format.

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

Vault constructs `VaultMetadata` internally on `Put` — the caller does not provide it. Vault derives each field:

- `FetchedAt` — `time.Now()`
- `Source` — derived from the namespace (same derivation logic as Manifold)
- `File` — derived from the namespace (standalone → `arrow.yaml`, quiver-scoped → `{auid}.yaml`)
- `OS` — from the Vault's configured target OS
- `EvictionTTL` — from the Vault's configured TTL

### 4.4 Staleness Check

Staleness is checked internally by `Get` operations and surfaced via `ErrStale`:

```go
func (v *Vault) isStale(meta VaultMetadata) bool {
    ttl, err := time.ParseDuration(meta.EvictionTTL)
    if err != nil {
        return true // unparseable TTL = always stale
    }
    return time.Since(meta.FetchedAt) > ttl
}
```

The caller never calls `isStale` directly — `Get` returns `ErrStale` when the entry's TTL has expired. The entry and home path are still returned alongside the error so the caller can use the stale data as a fallback.

### 4.5 Indirect Dependencies Lifecycle

The `IndirectDependencies` field on `VaultEntry` follows a specific lifecycle:

| Event | IndirectDependencies state |
|-------|---------------------------|
| Manifest cached (add / Manifold fetch) | `nil` — DepTree has not run yet |
| Install completes (DepTree resolved) | Populated — all transitive deps computed |
| Re-install / update | Re-populated — DepTree runs again, deps may have changed |
| Eviction (TTL expired, re-fetch) | Preserved — the app layer re-populates after re-fetch if the arrow was previously installed |
| Uninstall (orphan detection) | Read — used alongside `Manifest.Dependencies` to determine which deps to check for orphan status |

#### Orphan Detection

During uninstall cleanup, the use case layer reads `Manifest.Dependencies` and `IndirectDependencies` from the Vault entry of the arrow being uninstalled. It then scans all other Vault entries to determine if each dep is referenced by any other installed arrow. This scan is O(arrows × deps) — acceptable for the expected scale (tens of arrows, not thousands). See `deptree.md` §Uninstall Flow for the full sequence.

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
// Update Vault entry: svc.vault.PutArrow(ctx, root, manifest, indirect)
```

The app layer also updates the Vault entries of each **dependency** with their own `indirect_dependencies` as a side effect of the install — since DepTree has already resolved the full graph, the data is available at no extra cost.

---

## 5. Keying

Entries are keyed by **namespace only**. The OS is determined at install time and does not change for a given Quiver instance. The `OS` field in metadata is informational — it records which OS the manifest was resolved for but does not participate in cache lookup.

---

## 6. Disk Layout

### 6.1 Directory Structure

All namespaces share a unified tree under `~/.quiver/namespaces/`:

```
~/.quiver/namespaces/github.com/valve/steamcmd/arrow.json
~/.quiver/namespaces/github.com/char2cs/gaming.quiver/cs2/arrow.json
~/.quiver/namespaces/github.com/char2cs/gaming.quiver/quiver.json
```

The namespace segments map directly to directory segments. Arrows and quivers coexist in the same tree — the distinction is the filename (`arrow.json` vs `quiver.json`).

### 6.2 Path Mapping

| Type | Namespace | Home Directory | Manifest File |
|------|-----------|----------------|---------------|
| Standalone Arrow | `github.com/valve/steamcmd` | `namespaces/github.com/valve/steamcmd/` | `arrow.json` |
| Quiver Arrow | `github.com/char2cs/gaming.quiver/cs2` | `namespaces/github.com/char2cs/gaming.quiver/cs2/` | `arrow.json` |
| Quiver | `github.com/char2cs/gaming.quiver` | `namespaces/github.com/char2cs/gaming.quiver/` | `quiver.json` |

### 6.3 Arrow + Quiver Coexistence

A namespace that is both a Quiver and an Arrow naturally has both files in the same home directory:

```
~/.quiver/namespaces/github.com/char2cs/gaming.quiver/
  quiver.json       # Quiver manifest
  arrow.json        # Arrow manifest (if the Quiver is also an Arrow)
```

No collision. No special logic. The files are independent.

### 6.4 Home Directory Contents

The home directory is shared between Vault (manifest file) and the Wizard (installed artifacts):

```
~/.quiver/namespaces/github.com/valve/steamcmd/
  arrow.json         # Vault-owned — manifest + metadata
  steamcmd.sh        # Wizard-owned — installed by lifecycle steps
  linux32/           # Wizard-owned — runtime artifacts
  ...
```

Vault only ever reads and writes `arrow.json` or `quiver.json`. Everything else in the directory is managed by the Wizard's lifecycle steps. They coexist without interference.

---

## 7. Operations

### 7.1 Get

1. Compute path: `basePath/{namespace}/arrow.json` (or `quiver.json`)
2. Read file
3. If file does not exist → return `nil, "", ErrNotCached`
4. JSON unmarshal into `VaultEntry` or `QuiverVaultEntry`
5. Check staleness via `isStale(entry.Metadata)`
6. If stale → return `entry, homePath, ErrStale`
7. If fresh → return `entry, homePath, nil`

The home path is always `basePath/{namespace}/`.

### 7.2 Put

1. Compute home path: `basePath/{namespace}/`
2. Create directory tree if it does not exist (`MkdirAll`)
3. Construct `VaultMetadata` internally (fetched_at, source, file, os, ttl)
4. JSON marshal the entry (metadata + manifest)
5. Write to a temporary file in the same directory, then rename to `arrow.json` or `quiver.json` (atomic write)
6. Return the home directory path

Atomic write prevents partial reads if a Get happens concurrently.

### 7.3 Delete

1. Compute home path: `basePath/{namespace}/`
2. Remove the manifest file (`arrow.json` or `quiver.json`)
3. Check if the other manifest file exists in the same directory
4. If the other file exists → done (leave the directory intact)
5. If no other manifest file exists → `RemoveAll` on the home directory
6. If the file does not exist, return nil (idempotent)

This ensures that deleting an Arrow does not destroy a coexisting Quiver's manifest, and vice versa. The directory is only fully removed when both are gone.

---

## 8. Error Types

```go
var (
    // ErrNotCached indicates no entry exists in the Vault for the given namespace.
    ErrNotCached = errors.New("vault: entry not found")

    // ErrStale indicates the entry exists but its TTL has expired.
    // The entry and home path ARE still returned alongside this error
    // so the caller can use the stale data as a fallback.
    ErrStale = errors.New("vault: entry is stale")
)
```

All other errors (disk I/O failures, JSON marshal/unmarshal failures) propagate as wrapped errors with context.

---

## 9. Concurrency

### 9.1 Strategy

Per-namespace mutex using `sync.Map`:

```go
type Vault struct {
    basePath string     // ~/.quiver/namespaces/
    ttl      string     // e.g. "48h" — same for arrows and quivers
    os       string     // target OS for metadata
    locks    sync.Map   // map[string]*sync.Mutex
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

### 9.3 Vault and Wizard Coexistence

Since `Put` writes only `arrow.json`/`quiver.json` via atomic rename and the Wizard operates on application files, they do not interfere. `Delete` (which may `RemoveAll`) is only called after the Wizard has finished — the app layer ensures this by calling `Delete` only after `EndExecution{_uninstall, success}`.

---

## 10. Configuration

Vault receives its configuration at construction time:

```go
func New(basePath string, ttl string, os string) *Vault
```

- `basePath` — root directory for all namespaces (`~/.quiver/namespaces/`)
- `ttl` — eviction TTL applied to all entries (e.g., `"48h"`)
- `os` — target OS written to metadata (e.g., `"linux"`)

The TTL is the same for arrows and quivers. Vault uses these values when constructing `VaultMetadata` on `Put` and when checking staleness on `Get`.

---

## 11. App Layer Composition

Vault is not called in isolation. The app layer composes Vault with Manifold in a **cache-first pattern**:

### 11.1 Manifest Resolution (Cache-First)

```go
func (svc *ArrowService) resolveManifest(ctx context.Context, ns Namespace, os string) (*ArrowManifest, string, error) {
    // 1. Check Vault
    entry, homePath, err := svc.vault.GetArrow(ctx, ns)

    switch {
    case err == nil:
        // Fresh cache hit — fast path
        return entry.Manifest, homePath, nil

    case errors.Is(err, vault.ErrStale):
        // Entry exists but TTL expired — try refresh
        manifest, fetchErr := svc.manifold.ResolveArrow(ctx, ns, os)
        if fetchErr != nil {
            // Manifold failed — graceful degradation: use stale entry
            // log.Warn("using stale cache for %s, fetch failed: %v", ns, fetchErr)
            return entry.Manifest, homePath, nil
        }
        // Fresh manifest — overwrite arrow.json (artifacts untouched)
        homePath, _ = svc.vault.PutArrow(ctx, ns, manifest, nil)
        return manifest, homePath, nil

    case errors.Is(err, vault.ErrNotCached):
        // Never cached — full fetch from Manifold
        manifest, fetchErr := svc.manifold.ResolveArrow(ctx, ns, os)
        if fetchErr != nil {
            return nil, "", fetchErr
        }
        // Allocate home directory and persist
        homePath, _ = svc.vault.PutArrow(ctx, ns, manifest, nil)
        return manifest, homePath, nil

    default:
        return nil, "", err
    }
}
```

This preserves the graceful degradation behavior from the original Manifold spec: if the remote fetch fails but a stale cached copy exists, the stale copy is returned with a logged warning.

### 11.2 DepTree Resolver Callback

The resolver callback passed to DepTree uses the same cache-first pattern. It resolves a manifest and extracts the dependency list:

```go
resolver := func(ctx context.Context, ns Namespace) ([]Namespace, error) {
    manifest, _, err := svc.resolveManifest(ctx, ns, os)
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
    entry, _, err := svc.vault.GetArrow(ctx, ns)
    if err != nil && !errors.Is(err, vault.ErrStale) {
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

    _, err = svc.vault.PutArrow(ctx, ns, entry.Manifest, indirect)
    return err
}
```

### 11.4 Uninstall Cleanup

After a successful uninstall, the app layer calls `DeleteArrow` to clean up the home directory:

```go
// After EndExecution{_uninstall, success}
if outcome == ExecutionOutcomeSuccess {
    _ = svc.vault.DeleteArrow(ctx, namespace)
}
```

---

## 12. Constraints

- **No Asynx knowledge** — Vault is pure infrastructure. It does not emit events or commands.
- **No network I/O** — Vault only reads and writes to the local filesystem.
- **No manifest parsing** — Vault stores and retrieves pre-assembled domain objects. It does not understand YAML, schemas, or business rules.
- **Owns home directory allocation** — Vault is the single source of truth for namespace-to-path mapping. No other module computes home directory paths.
- **Vault-owned file boundary** — Vault only reads and writes `arrow.json` and `quiver.json`. It never touches other files in the home directory.
- **App layer is the only caller** — no other layer imports `Vault`.
- **Idempotent deletes** — deleting a non-existent entry is not an error.

---

## 13. File Layout

```
internal/infrastructure/vault/
    vault.go    — Vault struct, New(), GetArrow, PutArrow, DeleteArrow, GetQuiver, PutQuiver, DeleteQuiver
    models.go   — VaultEntry, QuiverVaultEntry, VaultMetadata
    errors.go   — ErrNotCached, ErrStale
```

---

## 14. Open Questions

| # | Question | Default if unresolved |
|---|----------|-----------------------|
| 1 | Should Vault support a `Purge()` method to clear all cached entries? | Not in v0 — add if a "clear cache" user command is needed. |
| 2 | Should Vault validate the JSON against the current domain types on read (to handle schema drift after upgrades)? | No — if the JSON can't unmarshal into the current struct, the entry is treated as missing (`ErrNotCached`-equivalent). The app layer re-fetches via Manifold. |
| 3 | Should Vault support listing all cached namespaces? | Not in v0 — add if a cache inspection CLI is needed. |
