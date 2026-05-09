# Quiver Manifest v0 — Design Spec

## Overview

Quivers are curated collections of arrows — a discovery and authorship primitive. They are not execution units. A user can browse any quiver by namespace (Get), or bookmark it to their local collection (Follow). Playlists (future) handle composition and execution ordering.

This spec covers the complete redesign of the quiver manifest format and the system changes required to support it.

---

## 1. Manifest Format

### QUIVER.md / quiver.yaml

Quiver manifests support both formats, mirroring arrow's ARROW.md / arrow.yaml pattern. QUIVER.md is a markdown file with a fenced `` ```quiver `` code block. The resolver tries `QUIVER.md` first, then `quiver.yaml`.

### Schema (`quiver@v0`)

```yaml
schema: "quiver@v0"

metadata:
  name: "Gaming Quiver"
  version: "1.0.0"
  description: "Game servers and utilities curated by char2cs"
  url: "https://gaming.quiver.ar"
  maintainers:
    - "char2cs"
  tags:
    - "gaming"
    - "servers"
  media:
    icon: "https://example.com/icon.png"
    banner: "https://example.com/banner.png"

arrows:
  - path: servers/cs2
  - path: tools/minecraft
  - namespace: github.com/valve/steamcmd
  - github.com/valve/steamcmd        # string shorthand, treated as external
```

### Arrow Entries

Two forms, mutually exclusive per entry:

- **Local** — `path: servers/cs2`. The file lives inside the quiver's own repo. Namespace is derived from the quiver's namespace + the last path segment: `github.com/char2cs/gaming.quiver/cs2`.
- **External** — `namespace: github.com/valve/steamcmd`. Points to an arrow in a separate repo. String shorthand (bare string in the array) is also supported for external arrows — must be a fully qualified namespace (e.g., `github.com/valve/steamcmd`), not a bare name like `cs2`.

No `description` on arrow entries — that comes from the arrow's own manifest.

### Version Strategy

Two independent dimensions exist for both arrows and quivers:

- **`@ref`** (git tag) — the resolution and pinning identity. How the engine finds the manifest. Used by `InstalledConstraint` for range upgrades (`v2.*`). Controlled by the consumer.
- **`metadata.version`** (manifest field) — the author's declared display version (`"2.1.0"`, `"2024-Q1"`). Independent of the git tag. What the UI shows users.

They are not required to match. A repo tagged `release-jan-2026` can declare `version: "2.0.0"` — no conflict, different purposes.

**Local arrows have no git tags of their own.** They live inside the quiver repo and are resolved at the quiver's ref. Their `metadata.version` in the arrow manifest is their only version identity. Removing it would leave them with no version at all. Both arrow and quiver manifests keep the `version` field.

### Validation Rules (QuiverRuleset)

- `name`, `description`, `schema` are required.
- All arrow entries must have either `path` or `namespace`, not both and not neither.
- Derived namespaces must be unique across the full arrow list (local and external combined). Duplicate detection normalizes all entries to their final namespace before comparing.

---

## 2. Domain Changes (`internal/domain/quiver.go`)

```go
// Raw translator output — before namespace derivation
type QuiverArrowEntry struct {
    Path      string `yaml:"path"`
    Namespace string `yaml:"namespace"`
}

// Resolved — after manifold derivation
type QuiverArrow struct {
    Namespace Namespace
}

type QuiverManifest struct {
    Name        string
    Version     string
    Description string
    URL         string
    Maintainers []string
    Tags        []string
    Media       QuiverMedia
    Arrows      []QuiverArrow
}
```

`QuiverVaultEntry` gains `FailedArrows []Namespace` to track arrows that couldn't be resolved during Follow/Get.

---

## 3. Manifold (`internal/engine/manifold/`)

### Translator (`translator/quiver/v0/`)

- `types.go`: new `metadataV0` block + `[]arrowEntryV0`. Custom `UnmarshalYAML` for arrow entries — handles both string shorthand and object form.
- `schema.json`: updated to reflect new shape.
- `Map(data []byte)` stays pure — returns raw `[]QuiverArrowEntry` without derived namespaces.

### Ruleset split (`ruleset/`)

The `Ruleset` module is reorganized into two internal subpackages:

- `ruleset/arrow/` — all existing arrow rules (moved from `ruleset/rules/`)
- `ruleset/quiver/` — new quiver rules, starting with duplicate namespace detection

The public `Ruleset` interface gains quiver validation methods alongside the existing arrow ones. Consistent with `translator/arrow/` and `translator/quiver/` pattern.

### Manifold interface

New method added:

```go
ParseQuiver(data []byte, ns domain.Namespace) (*domain.QuiverManifest, error)
```

Translates, derives local arrow namespaces from `ns`, runs `QuiverRuleset`, returns the resolved manifest. `ResolveQuiver` becomes fetch + `ParseQuiver`.

`ResolveQuiver` in the resolver tries `QUIVER.md` then `quiver.yaml`.

---

## 4. Vault (`internal/engine/vault/`)

Changes:
- `QuiverVaultEntry` gains `FailedArrows []Namespace`.
- `PutQuiver` signature gains a `failedArrows []domain.Namespace` parameter.
- New `ListCachedQuivers(ctx) ([]domain.Namespace, error)` method — walks `namespacesPath` for `quiver.json` entries, returns all cached quiver namespaces regardless of follow state. Used by the list filter.

Local and external arrows are both stored as regular arrow vault entries via `PutArrow`. The vault has no concept of local vs external — it stores bytes by namespace.

**Auto-update**: `sweepQuivers` already evicts stale quiver entries on TTL. `sweepArrows` evicts stale arrow entries independently. Both re-cache on next access. No special mechanism needed.

---

## 5. Repository & Usecase

### Quiver Repository (`repositories/quiver/`)

Two distinct concerns:

**Follow/Unfollow** — asynx aggregate. Commands renamed to `FollowQuiver` / `UnfollowQuiver`. Aggregate stores only follow state (namespace + followed_at) — manifest lives in the vault, not the aggregate. Existence check before Follow (`ErrAlreadyExists`) and Unfollow (`ErrNotFound`).

**Get** — stateless cache-or-fetch. Works for any namespace, followed or not. Flow:
1. Check vault for cached quiver manifest
2. If miss/stale: get workdir from `vault.WorkDir`, clone repo there, call `manifold.ParseQuiver(bytes, ns)`, clean up clone
3. Return manifest + local arrow bytes map to usecase

**List** — reads followed namespaces from asynx projection, calls Get for each.

### Quiver Usecase (`usecases/quiver.go`)

Takes `arrowrepo.Arrow` as new dependency:

```go
func NewQuiverUsecase(
    repo   quiverrepo.Quiver,
    arrows arrowrepo.Arrow,
) QuiverUsecase
```

On Follow/Get, orchestrates arrow caching:
- Local arrows (bytes in hand): `arrowRepo.Seed(ctx, ns, bytes)`
- External arrows: `arrowRepo.ResolveManifest(ctx, ns)`
- Failures collected → calls `repo.UpdateFailedArrows(ctx, ns, failedArrows)` which updates the vault entry. The repo stores the quiver manifest itself during the Get fetch; the usecase updates the failed list after arrow resolution completes.

**Partial failure behavior**: if some arrows can't be resolved, proceed and cache what's available. Failed namespaces are stored in `QuiverVaultEntry.FailedArrows`. Get enrichment skips resolution for failed arrows and returns them with `resolved: false`.

**Auto-retry**: controlled by `config.arrows.auto_retry` (`enabled: true`, `retries: 3` by default). When enabled, retries apply in two places:
- **Follow/caching**: each failed `arrowRepo.Seed` or `arrowRepo.ResolveManifest` call is retried up to `retries` times before the arrow is added to `FailedArrows`.
- **Get enrichment**: each failed `arrowRepo.GetManifest` call during enrichment is retried up to `retries` times before the arrow is returned with `resolved: false`.

Gains new methods mirroring `ArrowUsecase`:
- `Seed(ctx, ns, data)` — cache a quiver manifest from raw bytes
- `GetManifest(ctx, ns)` — return raw manifest
- `ValidateManifest(ctx, data)` — validate against schema + QuiverRuleset

`Update` is removed — TTL + sweep handles automatic refresh.

---

## 6. API & DTOs (`internal/api/v0/`)

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v0/quivers` | List quivers (filter by `followed`) |
| `GET` | `/v0/quiver/:ns` | Get quiver — works for any namespace |
| `POST` | `/v0/quiver/:ns/follow` | Follow a quiver |
| `DELETE` | `/v0/quiver/:ns/follow` | Unfollow a quiver |
| `GET` | `/v0/quiver/:ns/manifest` | Get raw manifest |
| `POST` | `/v0/quiver/:ns/manifest` | Seed a quiver manifest |
| `POST` | `/v0/quiver/:ns/manifest/validate` | Validate a quiver manifest |

### List filter

`GET /v0/quivers?followed=true` — only followed quivers
`GET /v0/quivers?followed=false` — cached but not followed
`GET /v0/quivers` — all locally known quivers

The `followed=false` case requires `vault.ListCachedQuivers` — walks `namespacesPath` for `quiver.json` entries, subtracts the followed set.

### DTOs

```go
type QuiverDetailDTO struct {
    Namespace   string           `json:"namespace"`
    Name        string           `json:"name"`
    Version     string           `json:"version,omitempty"`
    Description string           `json:"description"`
    URL         string           `json:"url,omitempty"`
    Maintainers []string         `json:"maintainers"`
    Tags        []string         `json:"tags"`
    Media       QuiverMediaDTO   `json:"media,omitempty"`
    Arrows      []QuiverArrowDTO `json:"arrows"`
    Followed    bool             `json:"followed"`
}

type QuiverArrowDTO struct {
    Namespace   string `json:"namespace"`
    Resolved    bool   `json:"resolved"`
    Name        string `json:"name,omitempty"`
    Version     string `json:"version,omitempty"`
    Description string `json:"description,omitempty"`
}

type QuiverListItemDTO struct {
    Namespace   string   `json:"namespace"`
    Name        string   `json:"name"`
    Description string   `json:"description"`
    Tags        []string `json:"tags"`
    ArrowCount  int      `json:"arrow_count"`
    Followed    bool     `json:"followed"`
}
```

---

## 7. Integration Tests

Currently `ResolveQuiver` returns a hard error in the test resolver. Needs:
- Fixture quiver manifests (both QUIVER.md and quiver.yaml formats)
- Local arrow fixtures inside quiver fixture repos
- Test resolver support for quiver namespaces
- Test cases: Follow, Get (cached/uncached), List (followed/unfollowed filter), partial arrow failure

---

## Arrow Manifest Touch (`internal/engine/manifold/translator/arrow/`)

This PR also touches the arrow manifest. `metadata.version` stays as a string (no change to the field itself). The version strategy section above applies to arrows equally — the field is kept, confirmed as the author's display identity independent of the git ref. No schema or type changes needed on the arrow side.

---

## Out of Scope (v0)

- Playlist support (BASE64-encoded shareable composition links)
- Auto-retry backoff / jitter (basic retry count is in scope via config)
- Quiver-level version notifications
