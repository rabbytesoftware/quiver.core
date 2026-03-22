# Quiver — Manifold

## Overview

Manifold is the infrastructure module responsible for turning a `Namespace` into a fully parsed domain object (`Arrow` or `Quiver`). The app layer passes a namespace in and gets a domain object back — it has no knowledge of git, YAML, file paths, or caching.

Internally the module owns two concerns:

| Concern | Responsibility |
|---------|----------------|
| **Resolver** | Git-fetch the manifest file from a remote repository and save it to the local workdir |
| **Translator** | Read a YAML file from local disk and produce a domain object |

Both concerns live inside the same module. The app layer sees only the module's top-level interface.

---

## 1. Module Name

`manifold` — the manifest resolution module.

The package lives at `internal/infrastructure/manifold`.

---

## 2. Interface Contract

The app layer depends on a single interface:

```go
// ManifoldPort is the interface the app layer depends on.
// It is defined in the app layer — manifold implements it.
type ManifoldPort interface {
    // ResolveArrow returns a fully parsed ArrowManifest for the given namespace.
    // It fetches from remote if the local copy is missing or evicted.
    ResolveArrow(ctx context.Context, namespace Namespace) (*ArrowManifest, error)

    // ResolveQuiver returns a fully parsed QuiverManifest for the given namespace.
    // It fetches from remote if the local copy is missing or evicted.
    ResolveQuiver(ctx context.Context, namespace Namespace) (*QuiverManifest, error)
}
```

This is the **only** interface the app layer imports. No git types, no file paths, no YAML — just namespace in, domain object out.

---

## 3. Resolution Logic

### 3.1 Namespace → Git URL

Known platforms derive the clone URL directly from the namespace:

| Platform | Namespace | Clone URL |
|----------|-----------|-----------|
| `github.com` | `github.com/valve/steamcmd` | `https://github.com/valve/steamcmd` |
| `gitlab.com` | `gitlab.com/company/tools` | `https://gitlab.com/company/tools` |
| `bitbucket.org` | `bitbucket.org/team/repo` | `https://bitbucket.org/team/repo` |

The domain component of the namespace determines the platform. Custom domains are **out of scope** for now.

### 3.2 Namespace → File Path

Two cases based on the namespace form:

**Standalone Arrow** (`domain/user/repo` — no colon)

```
Namespace:  github.com/valve/steamcmd
Clone URL:  https://github.com/valve/steamcmd
File:       arrow.yaml (always at repo root)
```

**Arrow inside a Quiver** (`domain/user/repo/auid` — four segments)

```
Namespace:  github.com/char2cs/gaming.quiver/cs2
Clone URL:  https://github.com/char2cs/gaming.quiver
File:       cs2.yaml (at repo root)
```

**Quiver** (`domain/user/repo` — no colon, called via `ResolveQuiver`)

```
Namespace:  github.com/char2cs/gaming.quiver
Clone URL:  https://github.com/char2cs/gaming.quiver
File:       quiver.yaml (always at repo root)
```

The caller (app layer) determines the expected type by calling `ResolveArrow` vs `ResolveQuiver`. The resolver does not guess.

### 3.3 Fetch Strategy

The resolver does **not** clone the full repository. Using `go-git` (pure Go, no OS git dependency), it performs a **shallow clone (depth=1)** into an in-memory or temporary filesystem, reads the target file from the worktree, saves it to the local workdir, and discards the clone.

`go-git` does not support fetching a single file from a remote — the git protocol requires at minimum fetching pack objects for a commit. A depth-1 shallow clone is the cheapest operation that gives us file access. The clone is transient (not persisted) — only the extracted YAML file is kept.

```
1. Shallow clone (depth=1) into memory/tmpdir
2. Read target file from worktree (e.g. arrow.yaml, cs2.yaml)
3. Write file to local workdir
4. Discard the clone — nothing else is stored
```

This keeps fetches fast while working within git protocol constraints.

---

## 4. Internal Flow

When the app layer calls `ResolveArrow(ctx, namespace)`:

```
1. Check local workdir for cached manifest file
2. If cached file exists AND eviction date has not passed → skip to step 5
3. Resolve namespace → git URL + file path
4. Git-fetch the manifest file → save to local workdir with eviction metadata
5. Pass local file path to Translator
6. Translator reads YAML, validates schema, maps to ArrowManifest
7. Return *ArrowManifest
```

Same flow for `ResolveQuiver`, substituting `QuiverManifest`.

### 4.1 Local Storage Layout

The storage path maps the namespace directly to a directory structure. The namespace path segments become directory segments — no encoding needed.

```
github.com/valve/steamcmd                →  github.com/valve/steamcmd/
github.com/char2cs/gaming.quiver/cs2     →  github.com/char2cs/gaming.quiver/cs2/
github.com/char2cs/gaming.quiver         →  github.com/char2cs/gaming.quiver/
```

```
~/.quiver/arrows/github.com/valve/steamcmd/manifest.yaml
~/.quiver/arrows/github.com/valve/steamcmd/metadata.yaml

~/.quiver/arrows/github.com/char2cs/gaming.quiver/cs2/manifest.yaml
~/.quiver/arrows/github.com/char2cs/gaming.quiver/cs2/metadata.yaml

~/.quiver/quivers/github.com/char2cs/gaming.quiver/manifest.yaml
~/.quiver/quivers/github.com/char2cs/gaming.quiver/metadata.yaml
```

Each directory contains two files:
- `manifest.yaml` — the arrow/quiver manifest fetched from the remote repository
- `metadata.yaml` — eviction and provenance tracking

### 4.2 Metadata File

The `metadata.yaml` file follows the same versioned schema convention as all other YAML files in Quiver:

```yaml
schema: "metadata@v0"

metadata:
  fetched_at: "2026-03-22T14:30:00Z"
  source: "https://github.com/valve/steamcmd"
  file: "arrow.yaml"
```

| Field | Description |
|-------|-------------|
| `fetched_at` | ISO 8601 timestamp of the last successful fetch. Compared against `now` + TTL to determine staleness. |
| `source` | The git clone URL used to fetch. Used for re-fetching on eviction. |
| `file` | The original filename in the remote repo (`arrow.yaml`, `cs2.yaml`, `quiver.yaml`). Needed because the local copy is always saved as `manifest.yaml`. |

---

## 5. Caching & Eviction

### 5.1 Strategy

Caching is file-based — a manifest saved to the local workdir IS the cache. No separate cache layer.

The eviction check is simple: if `now - fetched_at > eviction_ttl`, the manifest is stale and must be re-fetched on next access.

### 5.2 Eviction TTLs

| Type | Default TTL | Config Key |
|------|-------------|------------|
| Arrow manifest | 48 hours | `config.manifold.arrow_ttl` |
| Quiver manifest | 12 hours | `config.manifold.quiver_ttl` |

Quiver manifests evict faster because they are catalogs — new Arrows can be added to a Quiver at any time and users should see updates quickly.

### 5.3 Configuration

TTLs are configurable via `internal/core/config`:

```yaml
# default.yaml addition
config:
  manifold:
    arrow_ttl: "48h"
    quiver_ttl: "12h"
    fetch_timeout: "30s"
```

```go
// Config struct addition
type Manifold struct {
    ArrowTTL     string `yaml:"arrow_ttl"`      // e.g. "48h"
    QuiverTTL    string `yaml:"quiver_ttl"`      // e.g. "12h"
    FetchTimeout string `yaml:"fetch_timeout"`   // e.g. "30s"
}
```

### 5.4 Fetch Timeout

Every git operation (shallow clone) is wrapped with a deadline derived from `config.manifold.fetch_timeout` (default: 30 seconds). If the caller's `ctx` already has a shorter deadline, the shorter one wins. This prevents hung connections from blocking the app layer indefinitely.

### 5.5 Force Refresh

The app layer may need to force a refresh (e.g., user manually requests update). This is handled by a context value or a separate method — left to implementation. The spec only requires that the eviction mechanism is bypassable.

### 5.6 Eviction on Fetch Failure

If the remote fetch fails but a cached (stale) copy exists, the resolver returns the stale copy and logs a warning. A stale manifest is better than no manifest. If no cached copy exists, the error propagates.

---

## 6. Error Types

All errors are defined in the manifold package. The app layer switches on these types for user-facing messages.

```go
var (
    // ErrNotFound — the manifest file does not exist in the remote repository.
    ErrNotFound = errors.New("manifold: manifest not found")

    // ErrInvalidManifest — the YAML file exists but fails schema validation or parsing.
    ErrInvalidManifest = errors.New("manifold: invalid manifest")

    // ErrUnsupportedPlatform — the namespace domain is not a known git platform.
    ErrUnsupportedPlatform = errors.New("manifold: unsupported platform")

    // ErrFetchFailed — git transport error (network, auth, timeout).
    ErrFetchFailed = errors.New("manifold: fetch failed")
)
```

Errors wrap context via `fmt.Errorf("...: %w", ErrNotFound)` so callers can use `errors.Is`.

---

## 7. Git Client

The module uses [`go-git`](https://github.com/go-git/go-git) — a pure Go git implementation. **No OS-level git binary is required.**

The git client is an internal detail of the manifold package. It is not exposed through the `ManifoldPort` interface. The app layer has zero knowledge of git.

The resolver only needs read access to public repositories. Authentication (for private repos) is out of scope for now but the architecture does not preclude adding it later.

---

## 8. Translator (Internal)

The Translator is the second concern inside manifold. It already exists at `internal/infrastructure/translator` and handles:

1. Reading a YAML file from a local path
2. Extracting the `schema:` field to determine version (`arrow@v0`, `quiver@v0`)
3. Validating against the JSON schema for that version
4. Mapping to the domain type via the versioned mapper

The Translator is used **only** by the Resolver — it is not exposed through `ManifoldPort`. The existing Translator code moves into (or is imported by) the manifold package.

---

## 9. Constraints

- **No Asynx knowledge** — manifold is pure infrastructure. It does not emit events or commands.
- **App layer is the only caller** — no other layer imports `ManifoldPort`.
- **No git internals leak** — the interface takes `Namespace`, returns domain types. Period.
- **No custom domain resolution** — only `github.com`, `gitlab.com`, `bitbucket.org` for now.
- **No persisted clones** — shallow clone is transient; only the extracted manifest file is kept.
- **Pure Go** — no shell-out to `git`, no OS dependency beyond what Go itself provides.

---

## 10. Open Questions

| # | Question | Default if unresolved |
|---|----------|-----------------------|
| 1 | Should `ResolveArrow` on a Quiver-hosted namespace (`domain/user/repo/auid`) also verify that the Quiver manifest lists the AUID? Or just fetch `{auid}.yaml` blindly? | Fetch blindly — the Quiver catalog is a discovery aid, not an access control layer. |
| 2 | Should the module expose a `Prefetch(namespaces []Namespace)` for batch resolution (e.g., resolving all dependencies in parallel)? | Not in v0 — add when dependency resolution is specced. |
| 3 | Where does the Translator source code live — does it move into `manifold/`, or does `manifold` import it as-is? | Implementation decision. Either works — the spec only requires that Translator is not exposed through `ManifoldPort`. |
