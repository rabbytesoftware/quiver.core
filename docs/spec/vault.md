# Quiver — Vault

## Overview

Vault is the infrastructure module responsible for **allocating namespace home directories and persisting assembled domain objects** on disk. It stores `ArrowManifest` and `QuiverManifest` objects as JSON, wrapped in an envelope that carries metadata (fetch timestamp, eviction TTL).

Vault is keyed by `ArrowRef` for arrows — a `(namespace, version)` pair. Each installed version of an arrow gets its own subdirectory and its own `arrow.json`. Multiple versions of the same namespace coexist naturally under their shared namespace folder.

The app layer composes Vault and Manifold: call Manifold to fetch and assemble the manifest, then persist via Vault. On subsequent reads, check Vault first — call Manifold only on cache miss or stale entry.

Cross-references: [versioning.md](arrow/v0/versioning.md) · [domain.md](domain.md) · [deptree.md](deptree.md)

---

## 1. Module Name

`vault` — the namespace home directory and manifest persistence module.

The package lives at `internal/infrastructure/vault`.

---

## 2. Interface Contract

```go
type Vault interface {
    // PutArrow allocates a home directory for the ArrowRef (if it doesn't exist),
    // persists the ArrowManifest as arrow.json, and returns the home directory path.
    // Overwrites any existing arrow.json for this ref.
    // indirectDeps may be nil (pre-install) or populated (post-install, after DepTree resolution).
    PutArrow(ctx context.Context, ref ArrowRef, manifest *ArrowManifest, indirectDeps []ArrowRef) (string, error)

    // GetArrow retrieves a cached ArrowManifest for the given ArrowRef.
    // Returns the entry, the home directory path, and an error.
    //
    // Error semantics:
    //   - nil:          fresh cache hit (TTL not expired)
    //   - ErrStale:     entry exists but TTL expired — entry and path ARE still returned
    //   - ErrNotCached: no entry exists for this ref
    GetArrow(ctx context.Context, ref ArrowRef) (*VaultEntry, string, error)

    // ListVersions returns all installed version strings for a given namespace.
    // Each string is a subdirectory name under the namespace folder that contains
    // an arrow.json — e.g. ["latest", "v1.2.3", "v1.4.0"].
    // Returns an empty slice (not an error) if no versions are installed.
    ListVersions(ctx context.Context, ns Namespace) ([]string, error)

    // DeleteArrow removes the arrow.json for the given ArrowRef.
    // If the version subdirectory is now empty, removes it.
    // If the namespace folder has no remaining version subdirectories and no quiver.json, removes it too.
    // Idempotent — returns nil if the entry does not exist.
    DeleteArrow(ctx context.Context, ref ArrowRef) error

    // PutQuiver allocates a home directory for the namespace (if it doesn't exist),
    // persists the QuiverManifest as quiver.json, and returns the home directory path.
    PutQuiver(ctx context.Context, ns Namespace, manifest *QuiverManifest) (string, error)

    // GetQuiver retrieves a cached QuiverManifest for the given namespace.
    // Same error semantics as GetArrow.
    GetQuiver(ctx context.Context, ns Namespace) (*QuiverVaultEntry, string, error)

    // DeleteQuiver removes the quiver.json for the given namespace.
    // If no arrow version subdirectories exist in the same folder, removes the namespace folder.
    // Idempotent — returns nil if the entry does not exist.
    DeleteQuiver(ctx context.Context, ns Namespace) error
}
```

This is the **only** interface the app layer imports. Vault owns all path resolution — callers never compute home directory paths.

---

## 3. Storage Format

### 3.1 JSON Envelope

Each entry is a single JSON file containing the manifest and its metadata:

```json
{
  "metadata": {
    "fetched_at": "2026-04-17T14:30:00Z",
    "source": "https://github.com/valve/steamcmd",
    "file": "arrow.yaml",
    "eviction_ttl": "48h"
  },
  "manifest": {
    "name": "SteamCMD",
    "description": "Valve's command-line tool for installing and updating dedicated servers",
    "version": "0.0.1",
    "targets": { ... }
  },
  "indirect_dependencies": [
    { "namespace": "github.com/valve/steam-runtime", "version": "latest" }
  ]
}
```

The `manifest` field is the fully assembled `ArrowManifest` — targets parsed, validation passed, Overrideable fields intact. This is the raw manifest, not the compiled target. The Vault stores it for display and re-compilation; `CompiledTargets` live on the `Arrow` aggregate.

### 3.2 Why JSON, Not YAML

Same rationale as before: domain structs serialize cleanly to JSON, no YAML ambiguity, faster to parse. YAML is the author-facing format; JSON is the internal persistence format.

---

## 4. Data Model

### 4.1 `VaultEntry`

```go
type VaultEntry struct {
    Manifest             *ArrowManifest `json:"manifest"`
    Metadata             VaultMetadata  `json:"metadata"`
    IndirectDependencies []ArrowRef     `json:"indirect_dependencies,omitempty"`
}
```

`IndirectDependencies` holds all transitive dependencies that are not direct dependencies of the root arrow. `nil` before install (DepTree has not run); populated after install completes. Each entry is an `ArrowRef` carrying the exact version that was resolved.

### 4.2 `QuiverVaultEntry`

```go
type QuiverVaultEntry struct {
    Manifest *QuiverManifest `json:"manifest"`
    Metadata VaultMetadata   `json:"metadata"`
}
```

### 4.3 `VaultMetadata`

```go
type VaultMetadata struct {
    FetchedAt   time.Time `json:"fetched_at"`
    Source      string    `json:"source"`       // git clone URL used to fetch
    File        string    `json:"file"`          // original filename in repo (arrow.yaml, cs2.yaml, etc.)
    EvictionTTL string    `json:"eviction_ttl"`  // e.g. "48h"
}
```

Vault constructs `VaultMetadata` internally on `Put`. The `OS` field from the previous model is removed — compilation covers all 6 OS values at add time; there is no single-OS manifest resolution.

### 4.4 Staleness Check

```go
func (v *vault) isStale(meta VaultMetadata) bool {
    ttl, err := time.ParseDuration(meta.EvictionTTL)
    if err != nil {
        return true
    }
    return time.Since(meta.FetchedAt) > ttl
}
```

`Get` returns `ErrStale` when the TTL has expired. The entry and home path are still returned alongside the error — the caller may use the stale data as a fallback.

### 4.5 Indirect Dependencies Lifecycle

| Event | `IndirectDependencies` state |
|-------|------------------------------|
| Manifest cached (add / Manifold fetch) | `nil` — DepTree has not run yet |
| Install completes (DepTree resolved) | Populated — all transitive `ArrowRef`s computed |
| Re-install / update | Re-populated — DepTree runs again |
| Eviction (TTL expired, re-fetch) | Preserved — re-populated after re-fetch if arrow was previously installed |
| Uninstall (orphan detection) | Read — used to determine which `ArrowRef`s to check for orphan status |

---

## 5. Keying

Arrow entries are keyed by `ArrowRef` — a `(namespace, version)` pair. Two refs with the same namespace but different versions are independent entries at independent paths.

Quiver entries are keyed by `Namespace` alone — Quivers are not versioned.

`ListVersions(ctx, ns)` enumerates installed versions for a namespace by reading the version subdirectories under the namespace folder and checking for an `arrow.json` in each.

---

## 6. Disk Layout

### 6.1 Directory Structure

```
~/.quiver/namespaces/
  github.com/valve/steamcmd/
    latest/
      arrow.json       # VaultEntry for steamcmd@latest
      steamcmd.sh      # Wizard-owned artifacts
    v1.2.3/
      arrow.json       # VaultEntry for steamcmd@v1.2.3
      steamcmd.sh
  github.com/char2cs/gaming.quiver/
    quiver.json        # QuiverVaultEntry
    cs2/
      latest/
        arrow.json
        cs2_ds
```

The namespace folder is the container. Each version is a subdirectory. Wizard-owned artifacts (installed binaries, runtime files) live alongside `arrow.json` within the version folder.

### 6.2 Path Mapping

| Type | Ref | Home Directory | Manifest File |
|------|-----|----------------|---------------|
| Arrow (unversioned) | `github.com/valve/steamcmd` (latest) | `namespaces/github.com/valve/steamcmd/latest/` | `arrow.json` |
| Arrow (pinned) | `github.com/valve/steamcmd@v1.2.3` | `namespaces/github.com/valve/steamcmd/v1.2.3/` | `arrow.json` |
| Quiver Arrow | `github.com/char2cs/gaming.quiver/cs2` latest | `namespaces/github.com/char2cs/gaming.quiver/cs2/latest/` | `arrow.json` |
| Quiver | `github.com/char2cs/gaming.quiver` | `namespaces/github.com/char2cs/gaming.quiver/` | `quiver.json` |

### 6.3 Arrow + Quiver Coexistence

A Quiver's `quiver.json` lives directly in the namespace folder. Arrow version subdirectories live alongside it. No collision:

```
~/.quiver/namespaces/github.com/char2cs/gaming.quiver/
  quiver.json       # Quiver manifest
  cs2/              # Arrow scoped to this Quiver
    latest/
      arrow.json
```

### 6.4 Home Directory Contents

`INSTALL_PATH` for an arrow is the version subdirectory. All lifecycle steps execute with it as the working directory. Vault only ever reads and writes `arrow.json`. Everything else is Wizard-owned.

```
~/.quiver/namespaces/github.com/valve/steamcmd/v1.2.3/
  arrow.json         # Vault-owned
  steamcmd.sh        # Wizard-owned
  linux32/           # Wizard-owned
```

---

## 7. Operations

### 7.1 Get

1. Compute path: `basePath/{namespace}/{version}/arrow.json`
2. Read file
3. If file does not exist → return `nil, "", ErrNotCached`
4. JSON unmarshal into `VaultEntry`
5. Check staleness
6. If stale → return `entry, homePath, ErrStale`
7. If fresh → return `entry, homePath, nil`

Home path is `basePath/{namespace}/{version}/`.

### 7.2 Put

1. Compute home path: `basePath/{namespace}/{version}/`
2. `MkdirAll` on home path
3. Construct `VaultMetadata` internally
4. JSON marshal the entry
5. Write to temp file, rename to `arrow.json` (atomic write)
6. Return home path

### 7.3 Delete

1. Remove `arrow.json` from `basePath/{namespace}/{version}/`
2. If version directory is now empty → remove it
3. If namespace directory has no remaining version subdirectories and no `quiver.json` → remove namespace directory
4. If the file does not exist → return nil (idempotent)

### 7.4 ListVersions

1. Read directory entries under `basePath/{namespace}/`
2. For each subdirectory, check if `arrow.json` exists inside it
3. Return the names of subdirectories containing `arrow.json`

Returns `[]string{}` (not an error) if no versions are installed.

---

## 8. Error Types

```go
var (
    ErrNotCached = errors.New("vault: entry not found")
    ErrStale     = errors.New("vault: entry is stale")
)
```

All other errors (disk I/O, JSON failures) propagate as wrapped errors.

---

## 9. Concurrency

Per-`ArrowRef` mutex using `sync.Map`. Lock key is `ref.String()` (e.g. `"github.com/valve/steamcmd@v1.2.3"`).

| Operation | Lock Required |
|-----------|---------------|
| Get | No — atomic rename makes reads safe |
| Put | Yes — per-ref lock |
| Delete | Yes — per-ref lock |
| ListVersions | No — directory read only |

---

## 10. App Layer Composition

### 10.1 Manifest Resolution (Cache-First)

```go
func (svc *ArrowService) resolveManifest(ctx context.Context, ref ArrowRef) (*ArrowManifest, string, error) {
    entry, homePath, err := svc.vault.GetArrow(ctx, ref)

    switch {
    case err == nil:
        return entry.Manifest, homePath, nil

    case errors.Is(err, vault.ErrStale):
        manifest, fetchErr := svc.manifold.ResolveArrow(ctx, ref.Namespace)
        if fetchErr != nil {
            return entry.Manifest, homePath, nil // graceful degradation: use stale
        }
        homePath, _ = svc.vault.PutArrow(ctx, ref, manifest, nil)
        return manifest, homePath, nil

    case errors.Is(err, vault.ErrNotCached):
        manifest, fetchErr := svc.manifold.ResolveArrow(ctx, ref.Namespace)
        if fetchErr != nil {
            return nil, "", fetchErr
        }
        homePath, _ = svc.vault.PutArrow(ctx, ref, manifest, nil)
        return manifest, homePath, nil

    default:
        return nil, "", err
    }
}
```

### 10.2 DepTree Resolver Callback

```go
resolver := func(ctx context.Context, ref ArrowRef) ([]ArrowRef, error) {
    manifest, _, err := svc.resolveManifest(ctx, ref)
    if err != nil {
        return nil, err
    }
    target, ok := manifest.CompiledTargets[svc.os]  // from Arrow aggregate
    if !ok {
        return nil, fmt.Errorf("arrow %s does not support %s", ref, svc.os)
    }
    // merge tools and services — both are install-time dependencies
    deps := make([]ArrowRef, 0, len(target.Tools)+len(target.Services))
    deps = append(deps, target.Tools...)
    deps = append(deps, target.Services...)
    return deps, nil
}
```

### 10.3 Post-Install Vault Update

After DepTree resolves the full graph, the app layer updates the Vault entry with `indirect_dependencies`:

```go
indirect := []ArrowRef{}
directSet := toRefSet(directDeps) // tools + services from root's ResolvedTarget
for _, ref := range deptreeResult {
    if ref == root {
        continue
    }
    if !directSet[ref] {
        indirect = append(indirect, ref)
    }
}
svc.vault.PutArrow(ctx, root, manifest, indirect)
```

### 10.4 Uninstall Cleanup

```go
// After EndExecution{_uninstall, success}
if outcome == ExecutionOutcomeSuccess {
    _ = svc.vault.DeleteArrow(ctx, ref)
}
```

---

## 11. Constraints

- **No Asynx knowledge** — Vault is pure infrastructure.
- **No network I/O** — filesystem only.
- **No manifest parsing** — stores and retrieves pre-assembled domain objects.
- **Owns home directory allocation** — single source of truth for ref-to-path mapping.
- **Vault-owned file boundary** — only reads and writes `arrow.json` and `quiver.json`.
- **App layer is the only caller.**
- **Idempotent deletes.**

---

## 12. File Layout

```
internal/infrastructure/vault/
    vault.go    — Vault struct, New(), all interface methods
    models.go   — VaultEntry, QuiverVaultEntry, VaultMetadata
    errors.go   — ErrNotCached, ErrStale
```

---

## 13. Open Questions

| # | Question | Default if unresolved |
|---|----------|-----------------------|
| 1 | Should Vault support a `Purge()` method to clear all cached entries? | Not in v0. |
| 2 | Should Vault validate JSON against current domain types on read (schema drift)? | No — unmarshal failure treated as `ErrNotCached`; app layer re-fetches. |
| 3 | Should `ListVersions` include versions whose `arrow.json` is stale? | Yes — staleness is a cache concern, not a presence concern. Stale entries are still installed. |
