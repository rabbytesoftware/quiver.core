# Arrow Versioning Model

This document describes how Quiver manages multiple versions of the same Arrow. It
supplements [manifest.md](./manifest.md) and [deptree.md](../../deptree.md).

---

## Core principle

A namespace identifies a piece of software — not a specific version of it.
`github.com/valve/steamcmd` is always steamcmd. Version is metadata about an installation,
not part of the identity.

Versions are expressed as subdirectories within the Arrow's namespace folder. Quiver
maintains them; the filesystem layout makes the installed state immediately readable.

There is no semver range syntax and no SAT solver. Version constraints are glob patterns
matched against the arrow repo's git tags — Quiver filters, sorts, and picks the highest
match. This works with any tagging convention, not just semver.

The `latest` installation tracks HEAD of the default branch and is always current by
definition. Pinned versions and glob constraints are resolved to concrete versions at add
time and stored — Quiver never upgrades them automatically.

---

## Filesystem layout

```
~/.quiver/
  github.com/valve/steamcmd/
    latest/          ← unversioned installs (HEAD of default branch at install time)
    v1.2.3/          ← pinned to git tag v1.2.3
    v1.4.0/          ← pinned to git tag v1.4.0
  github.com/char2cs/myserver/
    latest/
```

`INSTALL_PATH` for an arrow always resolves to `~/.quiver/{namespace}/{version}/`.
Unversioned arrows use `latest` as the version segment.

Multiple arrows needing the same tool at the same version share one installation — same
namespace + same version = same directory = already installed.

---

## Manifest syntax

Version is declared at the dependency callsite — in `tools:` and `services:` — using an
`@ref` suffix. Three forms are supported:

| Form | Example | Meaning |
|------|---------|---------|
| No suffix | `steamcmd` | Latest — HEAD of default branch |
| Exact ref | `steamcmd@v1.2.3` | Exact git tag, branch, or commit SHA |
| Glob pattern | `steamcmd@v1.*` | Resolved at add time — latest tag matching the pattern |

```yaml
tools:
  - github.com/valve/steamcmd              # unversioned — latest/
  - github.com/valve/steamcmd@v1.2.3      # exact pin
  - github.com/valve/steamcmd@v1.*        # latest v1.x tag
  - github.com/valve/steamcmd@v1.2.*      # latest v1.2.x patch

services:
  - github.com/char2cs/myapp/database@v2.*
```

Glob patterns use standard `*` wildcard (matches any sequence of characters within a
tag segment). A `@ref` containing `*` is always treated as a glob; everything else is
treated as a literal git ref and used directly.

---

## Glob constraint resolution

When a `@ref` contains `*`, Quiver resolves it to a concrete version at `arrow.Add` time.
The raw pattern is preserved in the manifest (for re-resolution on `quiver update`); the
concrete resolved version is what gets stored in the vault and Arrow aggregate.

### Algorithm

```
1. git ls-remote --tags <repo>
   → collect all tag names

2. Filter tags by glob pattern
   e.g. "v1.*" → [v1.0.0, v1.2.3, v1.4.0]

3. Sort descending:
   a. If all matching tags look like semver (stripping leading "v"):
      → semver sort — v1.4.0 > v1.2.3 > v1.0.0
   b. Otherwise:
      → lexicographic sort

4. Pick the first (highest) — e.g. v1.4.0

5. Proceed as if the manifest declared @v1.4.0 exactly
```

If no tags match the pattern, the arrow is rejected at add time:

> Error: `tools: github.com/valve/steamcmd@v1.* — no git tags match pattern "v1.*"`

### What gets stored

| Location | Value |
|----------|-------|
| Raw manifest (Vault) | Pattern as authored — `@v1.*` |
| Arrow aggregate (`ArrowVersion`) | Concrete resolved version — `v1.4.0` |
| `ArrowRef` in `ResolvedTarget.Tools` | Concrete resolved version — `v1.4.0` |

The pattern never appears in runtime-facing data structures — only the concrete version does.

### Resolution timing

Constraint resolution happens at `arrow.Add` time, before `SelectTarget` runs. The app
layer resolves all glob patterns in `tools:` and `services:` to concrete `ArrowRef`s, then
passes the resolved refs into the target compilation pipeline. `SelectTarget` itself remains
a pure function — it never makes network calls.

### `quiver update` behavior

`quiver update github.com/valve/steamcmd@v1.*` re-runs the resolution algorithm. If the
latest `v1.*` tag has changed (e.g. `v1.5.0` was released), Quiver installs `v1.5.0/` and
migrates the Arrow's version entry. The old `v1.4.0/` subdirectory is removed if no other
arrow depends on it.

---

## Export references

Export references in step commands are unchanged regardless of version:

```yaml
command: ${github.com/valve/steamcmd.INSTALL_PATH}/${github.com/valve/steamcmd.steamcmd} +app_update 730 +quit
```

The manifest author never writes the version into a `${...}` reference. Quiver knows
which version the requesting arrow declared in `tools:` and resolves both the export value
and `INSTALL_PATH` against the correct version subdirectory at interpolation time. The
manifest stays version-transparent.

Export values are static relative paths (e.g. `./steamcmd.sh`) — they are never absolute
paths and never contain `${VAR}` tokens. The absolute path is always constructed by the
consumer by combining `${namespace.INSTALL_PATH}` with the named export. See
[manifest.md §6.3](./manifest.md#63-exports----named-values-exposed-to-dependents).

---

## Direct vs. dependency installs

Each installed version carries a `DirectInstall` flag — whether the user explicitly requested
this version, or whether Quiver installed it automatically to satisfy another arrow's
`tools:` or `services:` declaration.

```
Arrow: github.com/valve/steamcmd
  Versions:
    latest   DirectInstall: false  ← pulled in by cs2's tools:
    v1.2.3   DirectInstall: true   ← user ran: quiver add steamcmd@v1.2.3
```

`DirectInstall` is per-version, not per-namespace. The same namespace can have versions with
different origins simultaneously.

**Removal behavior:**

- `quiver remove github.com/valve/steamcmd@v1.2.3` — removes `v1.2.3` only if
  `DirectInstall: true` and no other installed arrow declares it in `tools:` or `services:`.
- `quiver remove github.com/valve/steamcmd` — removes all `DirectInstall: true` versions.
  Versions with `DirectInstall: false` are unaffected — they are owned by their dependents.
- When a dependent arrow is uninstalled, orphan detection removes any `DirectInstall: false`
  versions that are no longer referenced by any other installed arrow.

## Coexistence and sharing

| Scenario | Result |
|----------|--------|
| Arrow A and Arrow B both need `steamcmd@v1.2.3` | One install at `steamcmd/v1.2.3/`, shared |
| Arrow A needs `steamcmd@v1.*`, Arrow B needs `steamcmd@v1.*` | Both resolve to `v1.4.0` → one install, shared |
| Arrow A needs `steamcmd@v1.*` (→ `v1.4.0`), Arrow B needs `steamcmd@v1.2.*` (→ `v1.2.5`) | Two installs — `steamcmd/v1.4.0/` and `steamcmd/v1.2.5/` — no conflict |
| Arrow A needs `steamcmd@v1.*` (→ `v1.4.0`), Arrow B needs `steamcmd@v2.*` (→ `v2.1.0`) | Two installs — `steamcmd/v1.4.0/` and `steamcmd/v2.1.0/` — no conflict |
| Arrow A needs unversioned `steamcmd`, Arrow B needs `steamcmd@v1.2.3` | Two installs — `steamcmd/latest/` and `steamcmd/v1.2.3/` |

Version mismatches between arrows are never a conflict error. Each version gets its own
subdirectory. The dependency graph treats `(namespace, version)` as the node identity — see
[deptree.md](../../deptree.md) §2.

---

## Resolver behavior

The manifold resolver handles the `@ref` in two phases:

**Phase 1 — Constraint resolution (glob patterns only):**

If `@ref` contains `*`, the resolver calls `git ls-remote --tags` on the repository,
filters and sorts the results, and resolves to a concrete ref before fetching the manifest.
This phase is skipped for exact refs and unversioned requests.

**Phase 2 — Manifest fetch:**

The concrete ref (resolved or literal) is passed to the underlying fetcher:

| Fetcher | Behavior with concrete `@ref` |
|---------|-------------------------------|
| HTTP | Uses `@ref` as the `{branch}` segment in the raw URL template instead of `DefaultBranch` |
| Git | Sets `ReferenceName` in `CloneOptions` to the ref |

Without `@ref`, both fetchers use HEAD of the default branch.

---

## Vault

The Vault is keyed by `(namespace, version)` pairs. `github.com/valve/steamcmd` (latest)
and `github.com/valve/steamcmd@v1.2.3` are separate Vault entries with separate
`VaultMetadata` and separate compiled targets.

The `indirect_dependencies` field on a `VaultEntry` carries `ArrowRef` values (namespace +
version) rather than bare namespaces — so the full version graph is preserved for orphan
detection.

---

## Update cycle

### How version type determines update behavior

| Declared as | `quiver update` behavior |
|---|---|
| `steamcmd` (latest) | Re-fetch HEAD; run `update:` hook in-place inside `latest/`; fall back to uninstall+reinstall if absent |
| `steamcmd@v1.2.3` (exact) | Nothing — pinned, Quiver does not touch it |
| `steamcmd@v1.*` (glob) | Re-resolve constraint; if newer tag found, fresh-install the new version into a new subdirectory; orphan-check the old version |

### In-place update vs. version upgrade

These are distinct operations:

**In-place update** — `latest` slot, content changes. Runs `update:` hook inside the existing directory. State: `ready → updating → ready`. The directory identity does not change.

**Version upgrade** — glob resolves to a newer tag. Runs `install:` hook inside a **new** version subdirectory. The old subdirectory is orphan-checked and removed if nothing else depends on it. State follows the normal install cycle for the new version.

### Dependency update propagation

**`tools:` — no cascade**

A `tools:` dependency was consumed at install time — its output is on disk. Updating steamcmd does not require re-installing cs2-server. If cs2-server wants to use the new steamcmd to download game updates, that happens through cs2-server's own `update:` hook, which calls steamcmd again explicitly.

**`services:` — restart cascade**

When a `services:` dependency updates, Quiver coordinates the dependents:

```
1. Stop all running arrows that declare this service in services:
2. Run the service's update: hook (or uninstall+reinstall if absent)
3. Restart the stopped arrows
```

No `update:` hook runs on the dependents themselves — only a stop/restart. The service update is transparent to them.

### Update command surface

| Command | Behavior |
|---|---|
| `quiver update` | Update all `DirectInstall: true` arrows on `latest` or glob constraints |
| `quiver update cs2-server` | Update cs2-server specifically |
| `quiver update cs2-server@v1.*` | Update only the glob-constrained version |
| `quiver update steamcmd@v1.2.3` | No-op — exact pin |

Dependency-only arrows (`DirectInstall: false`) are never updated directly. They update as a side effect of updating the arrow that declared them.

---

## Uninstall and orphan detection

Orphan detection operates on `(namespace, version)` identity. If Arrow A (the only arrow
depending on `steamcmd@v1.2.3`) is uninstalled, the `v1.2.3/` subdirectory is removed.
The `v1.4.0/` subdirectory is unaffected even though it shares the `steamcmd/` parent.

If all version subdirectories under a namespace folder are removed, the namespace folder
itself is removed.

---

## Engine changes required

This model is not yet implemented. The following changes are needed:

| Component | Change |
|-----------|--------|
| `domain.Namespace` | Parse and carry `@ref` suffix; expose `Version() string` and bare `Namespace()` separately; detect glob patterns via `*` presence |
| `domain.ArrowRef` | New type: `{ Namespace, Version string }`. `Version` always holds a concrete resolved version — never a glob pattern |
| Constraint resolver | New: `ResolveConstraint(ctx, ns, pattern) (ArrowRef, error)` — calls `git ls-remote --tags`, filters by glob, sorts, returns concrete `ArrowRef` |
| Metadata path resolution | `INSTALL_PATH` = `~/.quiver/{namespace}/{version}/` |
| HTTP fetcher | Use concrete ref as `{branch}` instead of `DefaultBranch` when present |
| Git fetcher | Set `ReferenceName` in `CloneOptions` when ref is present |
| `arrow.Add` use case | Resolves glob constraints to concrete `ArrowRef`s before calling `SelectTarget` |
| `deptree.ResolverFunc` | Returns `[]ArrowRef` (concrete versions) instead of `[]Namespace` |
| Vault | Keyed by `ArrowRef`; `indirect_dependencies` carries `[]ArrowRef`; adds `ListVersions(ctx, Namespace) ([]string, error)` |
| Arrow aggregate | Namespace-keyed; holds `Versions map[string]ArrowVersion`; display metadata at namespace level from latest resolved manifest |
| `ArrowVersion` | New type: `{ CompiledTargets map[OS]ResolvedTarget, InstalledAt time.Time, DirectInstall bool }` |
| `arrowRuntime` aggregate | Keyed by `ArrowRef` — per-version lifecycle state, independent across versions |
| `quiver remove` use case | Checks `DirectInstall` before removing; orphan detection operates on `ArrowRef` pairs |
| `quiver update` use case | In-place update for `latest`; fresh install + orphan-check for glob upgrades; no-op for exact pins; restart cascade for `services:` dependents |
