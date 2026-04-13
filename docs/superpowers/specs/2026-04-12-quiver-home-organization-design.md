# Quiver Home Directory Organization

**Date:** 2026-04-12
**Branch:** fix/quiver-home-organization

---

## Problem

The `~/.quiver` home directory is disorganized:

- 5 database files dumped at the root with no grouping
- Event stores split inconsistently (`arrow-events.db`, `runtime-events.db`, `quiver-events.db` at root; `netbridge.db` one level deeper under `events/`)
- Catalog read models (`arrows.db`, `quivers.db`) mixed alongside event stores at root
- Config uses relative paths (`./config.yaml`, `./db`, etc.) that resolve to CWD at runtime, not to the Quiver home
- No single authoritative source of truth for where each file type lives

---

## Target Directory Structure

```
~/.quiver/
├── namespaces/              ← arrow workdirs + cached manifests (first-class citizen)
│   └── github.com/
│       └── <user>/
│           └── <repo>/
└── state/
    ├── events/              ← all event stores (append-only, source of truth)
    │   ├── arrow.db
    │   ├── runtime.db
    │   ├── quiver.db
    │   └── netbridge.db
    └── store/               ← catalog read models (derived, reconstructable)
        ├── arrows.db
        └── quivers.db
```

`namespaces/` is a first-class directory: it is both the working directory for running arrows and the location of cached manifests (`arrow.json`, `quiver.json`). It sits at the root alongside `state/`, not nested inside it.

Old files at previous paths are left in place — no migration. Quiver is pre-production.

---

## Design

### 1. `OsValue[T]` — new type in `internal/core/metadata/`

A new generic type that mirrors the `Overrideable[T]` pattern from the domain but resolves by OS only (not OS/arch), and lives entirely within the metadata package. No dependency on the domain type.

```
File: internal/core/metadata/osvalue.go
```

Behaviour:
- In YAML: accepts either a scalar (default only) or a map with a `"default"` key plus optional OS keys (`"windows"`, `"linux"`, `"darwin"`)
- `Resolve() T` — returns the value for `runtime.GOOS`, falling back to `Default`
- `UnmarshalYAML` handles both scalar and map YAML nodes

### 2. `metadata.yaml` — replace `variables` section

Remove `DEFAULT_CONFIG_PATH` and `QUIVER_HOME` from `variables`. Add a top-level `paths` section:

```yaml
paths:
  home:
    default: "~/.quiver"
    windows: 'C:\Users\{{USER}}\Documents\.quiver'
  events: "{{home}}/state/events"
  store: "{{home}}/state/store"
  namespaces: "{{home}}/namespaces"
  config: "{{home}}/config.yaml"
```

Template variables:
- `{{home}}` — resolved home path (OS-specific, user home dir expanded)
- `{{USER}}` — current OS username (used in Windows home path only)

Only `home` uses `OsValue[string]` (scalar-or-map). All derived paths are plain `string` with template variables.

### 3. `metadata.go` — updated structs and public interface

**Remove:**
- `QuiverHome` struct
- `DefaultConfigPath` field from `Variables`
- `GetDefaultConfigPath()`, `GetQuiverHome()`, `currentUsername()`

**Add:**

```go
type Paths struct {
    Home       OsValue[string] `yaml:"home"`
    Events     string          `yaml:"events"`
    Store      string          `yaml:"store"`
    Namespaces string          `yaml:"namespaces"`
    Config     string          `yaml:"config"`
}

type Metadata struct {
    // ...existing fields...
    Paths Paths `yaml:"paths"`
}
```

`Variables` loses the `QuiverHome` and `DefaultConfigPath` fields entirely. If it becomes empty after that removal, it is deleted too.

Public getters (all resolve templates internally, callers receive ready-to-use absolute paths):

```go
func GetHomePath() string
func GetEventsPath() string
func GetStorePath() string
func GetNamespacesPath() string
func GetConfigPath() string
```

Private resolution:
- `resolveHome() string` — calls `OsValue.Resolve()`, expands `~` to `os.UserHomeDir()`, substitutes `{{USER}}`
- `resolvePath(tmpl, home string) string` — replaces `{{home}}` with the resolved home

### 4. `config.go` — merge semantics + path ownership

**Path ownership:** `GetConfigPath()` moves to metadata. Remove `GetConfigPath()` and `ConfigExists()` from the config package.

**Merge semantics:** replace the current all-or-nothing unmarshal with a two-step approach:
1. Start from `getDefaultConfig()` (embedded defaults fully populated)
2. Read `metadata.GetConfigPath()` and `yaml.Unmarshal` into the already-populated struct — yaml.v3 only sets fields present in the file, so absent fields keep their defaults

This enables partial user configs (e.g., overriding only `api.port` while keeping all other defaults).

### 5. Call-site updates

All callers stop hardcoding paths and use metadata getters instead:

| File | Old path | New call |
|---|---|---|
| `adapter/container.go` | `home + "/arrow-events.db"` etc. | `metadata.GetEventsPath()` |
| `engine/container.go` | custom `openEventStore` helper | `metadata.GetEventsPath()` |
| `app/arrow/builder.go` | `home + "/arrows.db"` | `metadata.GetStorePath()` |
| `app/quiver/builder.go` | `home + "/quivers.db"` | `metadata.GetStorePath()` |
| `engine/vault/store.go` | `GetQuiverHome() + "/namespaces"` fallback | `metadata.GetNamespacesPath()` fallback |

The `openEventStore` helper in `engine/container.go` is deleted. Its logic (ensure dir exists, open SQLite) is inlined or simplified now that the path comes from metadata.

---

## What is NOT changed

- `overrideableV0[T]` in the arrow manifest translator — untouched, it serves a different purpose
- `Overrideable[T]` in the domain — untouched
- The `namespaces/` internal layout (`<host>/<user>/<repo>/`) — unchanged
- Arrow manifest format, API, or any engine behaviour
