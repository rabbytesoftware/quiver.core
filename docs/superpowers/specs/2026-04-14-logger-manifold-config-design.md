# Logger + Manifold Config Design

**Date:** 2026-04-14
**Branch:** fix/quiver-home-organization

---

## Changes

### 1. `metadata.yaml` + `metadata.go` — add `logs` path

New entry in the top-level `paths:` section:

```yaml
paths:
  ...
  logs: "{{home}}/logs"
```

`Paths` struct gains `Logs string`. New getter: `GetLogsPath() string`.

### 2. `internal/core/paths/paths.go` — add `Logs()`

```go
func Logs() (string, error) {
    return ensure(metadata.GetLogsPath())
}
```

Same pattern as `Events`, `Store`, `Namespaces`. `export_test.go` already covers `ensure`; one new test added for `Logs`.

### 3. `config.go` + `default.yaml` — rename, strip, add

**`default.yaml`** (full content):
```yaml
config:
  netbridge:
    enabled: true
    ephemeral_port_start: 49152
    ephemeral_port_end: 65535

  api:
    host: 0.0.0.0
    port: 40257

  logger:
    enabled: true
    level: info

  manifold:
    fetch_timeout: 30s
```

`arrows` and `database` sections removed — no production callers exist.

**`config.go` struct changes:**
- `Watcher` struct removed; replaced by `Logger struct { Enabled bool; Level string }`
- `Database` struct removed
- `Arrows` struct removed
- New `Manifold struct { FetchTimeout string }`
- `ConfigData` updated accordingly
- `GetWatcher()` removed → `GetLogger() Logger` added
- `GetDatabase()` removed
- `GetArrows()` removed
- `GetManifold() Manifold` added

### 4. `internal/core/logger/logger.go`

Signature: `Init(cfg config.Logger) func() error`

Log path: `paths.Logs()` — not from config.
Filename: `Quiver.log`.
Rotation rules (all hardcoded, not user-configurable):
- MaxSize: 5 MB
- Compress: true
- MaxAge: 0 (keep all rotated files)
- MaxBackups: 0 (no cap on count)

### 5. `internal/engine/container.go`

Remove `const manifoldFetchTimeout`. Parse from config at startup:

```go
cfg := config.GetManifold()
timeout, err := time.ParseDuration(cfg.FetchTimeout)
if err != nil {
    timeout = 30 * time.Second
}
```

### 6. `internal/core/core.go`

`config.GetWatcher()` → `config.GetLogger()`.

---

## What is NOT changed

- `lumberjack.v2` library — still used for rotation
- Logger output destinations (stdout + file when enabled, stderr only when disabled)
- Log format (JSON when file-enabled, text when stderr-only)
- `netbridge` and `api` config sections — unchanged
