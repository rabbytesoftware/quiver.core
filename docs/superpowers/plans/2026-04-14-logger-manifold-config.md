# Logger + Manifold Config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `logs` path to the metadata/paths system, rename `watcher` → `logger` in config (stripping stale fields), add a `manifold` config section with a configurable fetch timeout, and wire the logger to write `Quiver.log` files into `~/.quiver/logs/` with 5 MB rotation.

**Architecture:** Four sequential tasks — paths first (nothing depends on it yet), then config (defines types used by logger and engine), then logger (consumes both paths and config), then call-site wiring (consumes config). Each task produces a compilable state except Task 1→2 boundary where logger tests temporarily reference the old `config.Watcher` type.

**Tech Stack:** Go standard library, `gopkg.in/natefinch/lumberjack.v2`, `gopkg.in/yaml.v3`, `github.com/stretchr/testify`

---

## File Map

| Action | File |
|---|---|
| **Modify** | `internal/core/metadata/metadata.yaml` |
| **Modify** | `internal/core/metadata/metadata.go` |
| **Modify** | `internal/core/metadata/metadata_test.go` |
| **Modify** | `internal/core/paths/paths.go` |
| **Modify** | `internal/core/paths/paths_test.go` |
| **Modify** | `internal/core/config/default.yaml` |
| **Modify** | `internal/core/config/config.go` |
| **Modify** | `internal/core/config/config_test.go` |
| **Modify** | `internal/core/logger/logger.go` |
| **Modify** | `internal/core/logger/logger_test.go` |
| **Modify** | `internal/core/core.go` |
| **Modify** | `internal/engine/container.go` |

---

### Task 1: Add `logs` path to metadata and paths module

**Files:**
- Modify: `internal/core/metadata/metadata.yaml`
- Modify: `internal/core/metadata/metadata.go`
- Modify: `internal/core/metadata/metadata_test.go`
- Modify: `internal/core/paths/paths.go`
- Modify: `internal/core/paths/paths_test.go`

- [ ] **Step 1: Add `logs` to `metadata.yaml`**

In `internal/core/metadata/metadata.yaml`, add `logs` to the `paths:` section:

```yaml
paths:
  home:
    default: "~/.quiver"
    windows: 'C:\Users\{{USER}}\Documents\.quiver'
  events: "{{home}}/state/events"
  store: "{{home}}/state/store"
  namespaces: "{{home}}/namespaces"
  config: "{{home}}/config.yaml"
  logs: "{{home}}/logs"
```

- [ ] **Step 2: Add `Logs` field and `GetLogsPath()` to `metadata.go`**

Add `Logs string` to the `Paths` struct and add the getter. In `internal/core/metadata/metadata.go`:

```go
type Paths struct {
	Home       OsValue[string] `yaml:"home"`
	Events     string          `yaml:"events"`
	Store      string          `yaml:"store"`
	Namespaces string          `yaml:"namespaces"`
	Config     string          `yaml:"config"`
	Logs       string          `yaml:"logs"`
}
```

Add getter after `GetConfigPath()`:

```go
func GetLogsPath() string {
	return resolvePath(Get().Paths.Logs, resolveHome())
}
```

Update `defaultMetadata()` — add `Logs` field inside `Paths`:

```go
Paths: Paths{
	Home: OsValue[string]{
		Default: "~/.quiver",
		OS: map[string]string{
			"windows": `C:\Users\{{USER}}\Documents\.quiver`,
		},
	},
	Events:     "{{home}}/state/events",
	Store:      "{{home}}/state/store",
	Namespaces: "{{home}}/namespaces",
	Config:     "{{home}}/config.yaml",
	Logs:       "{{home}}/logs",
},
```

- [ ] **Step 3: Add `TestGetLogsPath_*` tests to `metadata_test.go`**

Add two tests after the existing `TestGetConfigPath_*` tests:

```go
func TestGetLogsPath_ContainsHome(t *testing.T) {
	assert.True(t, strings.HasPrefix(GetLogsPath(), GetHomePath()))
}

func TestGetLogsPath_EndsWithLogs(t *testing.T) {
	assert.True(t, strings.HasSuffix(GetLogsPath(), "logs"))
}
```

Also update `TestDefaultMetadata_PathsPopulated` to include `Logs`:

```go
func TestDefaultMetadata_PathsPopulated(t *testing.T) {
	d := defaultMetadata()
	assert.NotEmpty(t, d.Paths.Home.Default)
	assert.NotEmpty(t, d.Paths.Events)
	assert.NotEmpty(t, d.Paths.Store)
	assert.NotEmpty(t, d.Paths.Namespaces)
	assert.NotEmpty(t, d.Paths.Config)
	assert.NotEmpty(t, d.Paths.Logs)
}
```

- [ ] **Step 4: Run metadata tests**

```bash
cd /Users/char2cs/.superset/worktrees/quiver.core/fix/quiver-home-organization
go test ./internal/core/metadata/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Add `Logs()` to `internal/core/paths/paths.go`**

Add after `Namespaces()`:

```go
// Logs returns the absolute path to the logs directory,
// creating it if it does not exist.
func Logs() (string, error) {
	return ensure(
		metadata.GetLogsPath(),
	)
}
```

- [ ] **Step 6: Add `Logs` tests to `internal/core/paths/paths_test.go`**

Add after the existing `TestNamespaces_*` tests:

```go
func TestLogs_CreatesDir(t *testing.T) {
	ensureCreatesDir(t, paths.Logs)
}

func TestLogs_ReturnsAbsolutePath(t *testing.T) {
	got, err := paths.Logs()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(got))
}
```

Also add `paths.Logs()` to the concurrent test goroutine:

```go
func TestConcurrentCalls_NoRace(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = paths.Events()
			_, _ = paths.Store()
			_, _ = paths.Namespaces()
			_, _ = paths.Logs()
		}()
	}
	wg.Wait()
}
```

- [ ] **Step 7: Run paths tests**

```bash
go test ./internal/core/paths/... -v -race
```

Expected: all tests PASS, no race conditions.

- [ ] **Step 8: Commit**

```bash
git add internal/core/metadata/metadata.yaml internal/core/metadata/metadata.go internal/core/metadata/metadata_test.go internal/core/paths/paths.go internal/core/paths/paths_test.go
git commit -m "feat(paths): add logs path — GetLogsPath() and paths.Logs()"
```

---

### Task 2: Config — rename watcher→logger, remove stale fields, add manifold

**Files:**
- Modify: `internal/core/config/default.yaml`
- Modify: `internal/core/config/config.go`
- Modify: `internal/core/config/config_test.go`

- [ ] **Step 1: Rewrite `default.yaml`**

Replace the entire file with:

```yaml
# ? The default config are a collection of variables that
# ? can be changed by the user on runtime, but these remain
# ? as defaults when either not present or empty.

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

- [ ] **Step 2: Rewrite `config.go`**

Replace the entire file:

```go
package config

import (
	"context"
	_ "embed"
	"path/filepath"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/core/fns"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	yaml "gopkg.in/yaml.v3"
)

var (
	//go:embed default.yaml
	defaultConfigByte []byte
	config            *Config
	once              sync.Once
)

type Netbridge struct {
	Enabled            bool `yaml:"enabled"`
	EphemeralPortStart int  `yaml:"ephemeral_port_start"`
	EphemeralPortEnd   int  `yaml:"ephemeral_port_end"`
}

type API struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type Logger struct {
	Enabled bool   `yaml:"enabled"`
	Level   string `yaml:"level"`
}

type Manifold struct {
	FetchTimeout string `yaml:"fetch_timeout"`
}

type ConfigData struct {
	Netbridge Netbridge `yaml:"netbridge"`
	API       API       `yaml:"api"`
	Logger    Logger    `yaml:"logger"`
	Manifold  Manifold  `yaml:"manifold"`
}

type Config struct {
	Config ConfigData `yaml:"config"`
}

// Get returns the singleton config. It starts from the embedded defaults and
// overlays any fields present in the user's config.yaml at GetConfigPath().
// Fields absent from the user file keep their embedded default values.
func Get() *Config {
	once.Do(func() {
		config = getDefaultConfig()

		configPath := filepath.Clean(metadata.GetConfigPath())
		configBytes, err := fns.Read(context.Background(), configPath)
		if err != nil {
			return
		}

		if err := yaml.Unmarshal(configBytes, config); err != nil {
			config = getDefaultConfig()
		}
	})
	return config
}

func GetNetbridge() Netbridge {
	return Get().Config.Netbridge
}

func GetAPI() API {
	return Get().Config.API
}

func GetLogger() Logger {
	return Get().Config.Logger
}

func GetManifold() Manifold {
	return Get().Config.Manifold
}

func getDefaultConfig() *Config {
	cfg := &Config{}
	if err := yaml.Unmarshal(defaultConfigByte, cfg); err == nil {
		return cfg
	}
	return &Config{
		Config: ConfigData{
			Netbridge: Netbridge{
				Enabled:            true,
				EphemeralPortStart: 49152,
				EphemeralPortEnd:   65535,
			},
			API: API{
				Host: "0.0.0.0",
				Port: 40257,
			},
			Logger: Logger{
				Enabled: true,
				Level:   "info",
			},
			Manifold: Manifold{
				FetchTimeout: "30s",
			},
		},
	}
}

func resetForTesting() {
	config = nil
	once = sync.Once{}
}
```

- [ ] **Step 3: Rewrite `config_test.go`**

Replace the entire file:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet_ReturnsSingleton(t *testing.T) {
	resetForTesting()
	cfg := Get()
	require.NotNil(t, cfg)
	assert.Same(t, cfg, Get())
}

func TestGet_DefaultsPopulated(t *testing.T) {
	resetForTesting()
	cfg := Get()
	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Config.API.Host)
	assert.Positive(t, cfg.Config.API.Port)
	assert.NotEmpty(t, cfg.Config.Logger.Level)
	assert.NotEmpty(t, cfg.Config.Manifold.FetchTimeout)
	assert.Positive(t, cfg.Config.Netbridge.EphemeralPortStart)
	assert.Positive(t, cfg.Config.Netbridge.EphemeralPortEnd)
}

func TestGetNetbridge_FieldsAccessible(t *testing.T) {
	nb := GetNetbridge()
	_ = nb.Enabled
	_ = nb.EphemeralPortStart
	_ = nb.EphemeralPortEnd
}

func TestGetAPI_ValidValues(t *testing.T) {
	api := GetAPI()
	assert.NotEmpty(t, api.Host)
	assert.Positive(t, api.Port)
}

func TestGetLogger_ValidValues(t *testing.T) {
	lg := GetLogger()
	assert.NotEmpty(t, lg.Level)
}

func TestGetManifold_FetchTimeout_ParseableAsDuration(t *testing.T) {
	m := GetManifold()
	assert.NotEmpty(t, m.FetchTimeout)
	_, err := time.ParseDuration(m.FetchTimeout)
	assert.NoError(t, err, "FetchTimeout %q must be parseable by time.ParseDuration", m.FetchTimeout)
}

func TestGetDefaultConfig_NeverNil(t *testing.T) {
	require.NotNil(t, getDefaultConfig())
}

func TestGet_MissingFile_UsesDefaults(t *testing.T) {
	resetForTesting()
	cfg := Get()
	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Config.API.Host)
}

func TestGet_WithValidConfigFile_MergesOverrides(t *testing.T) {
	path := metadata.GetConfigPath()
	original, originalErr := os.ReadFile(path)
	t.Cleanup(func() {
		resetForTesting()
		if originalErr != nil {
			os.Remove(path)
		} else {
			os.WriteFile(path, original, 0644)
		}
	})

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))

	// Partial override — only api.host and api.port; all other fields keep defaults.
	require.NoError(t, os.WriteFile(path, []byte(`config:
  api:
    host: "test-host"
    port: 9999
`), 0644))

	resetForTesting()
	cfg := Get()

	require.NotNil(t, cfg)
	assert.Equal(t, "test-host", cfg.Config.API.Host)
	assert.Equal(t, 9999, cfg.Config.API.Port)
	// Logger defaults must survive the partial override.
	assert.NotEmpty(t, cfg.Config.Logger.Level)
}

func TestGet_WithInvalidYAML_FallsBackToDefaults(t *testing.T) {
	path := metadata.GetConfigPath()
	original, originalErr := os.ReadFile(path)
	t.Cleanup(func() {
		resetForTesting()
		if originalErr != nil {
			os.Remove(path)
		} else {
			os.WriteFile(path, original, 0644)
		}
	})

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte("not: [valid: yaml\x00"), 0644))

	resetForTesting()
	cfg := Get()

	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Config.API.Host)
}
```

- [ ] **Step 4: Run config tests**

```bash
go test ./internal/core/config/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/config/default.yaml internal/core/config/config.go internal/core/config/config_test.go
git commit -m "feat(config): rename watcher→logger, add manifold section, remove stale arrows/database fields"
```

---

### Task 3: Update logger to use `paths.Logs()` and hardcoded rotation

**Files:**
- Modify: `internal/core/logger/logger.go`
- Modify: `internal/core/logger/logger_test.go`

- [ ] **Step 1: Rewrite `logger.go`**

Replace the entire file:

```go
package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/core/paths"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

const (
	logFilename = "Quiver.log"
	logMaxSizeMB = 5
)

// Init configures slog.Default for the lifetime of the process.
// When cfg.Enabled is false, logs go to stderr only.
// When cfg.Enabled is true, logs go to both stdout and a rotating file
// under the Quiver logs directory (~/.quiver/logs/Quiver.log).
// Returns a shutdown function that closes the log file; call it before process exit.
func Init(cfg config.Logger) func() error {
	roller, handler := buildHandler(cfg)
	slog.SetDefault(slog.New(handler))
	return func() error {
		if roller != nil {
			return roller.Close()
		}
		return nil
	}
}

func buildHandler(cfg config.Logger) (*lumberjack.Logger, slog.Handler) {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}

	if !cfg.Enabled {
		return nil, slog.NewTextHandler(os.Stderr, opts)
	}

	logsPath, err := paths.Logs()
	if err != nil {
		return nil, slog.NewTextHandler(os.Stderr, opts)
	}

	roller := &lumberjack.Logger{
		Filename:  filepath.Join(logsPath, logFilename),
		MaxSize:   logMaxSizeMB,
		Compress:  true,
		LocalTime: true,
	}

	return roller, slog.NewJSONHandler(io.MultiWriter(os.Stdout, roller), opts)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug", "trace":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "fatal", "panic":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
```

- [ ] **Step 2: Rewrite `logger_test.go`**

Replace the entire file:

```go
package logger_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/core/logger"
	"github.com/rabbytesoftware/quiver/internal/core/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_DisabledConfig_DoesNotPanic(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	assert.NotPanics(t, func() {
		_ = logger.Init(config.Logger{Enabled: false, Level: "info"})
	})
}

func TestInit_EnabledConfig_CreatesLogFile(t *testing.T) {
	prev := slog.Default()

	logsPath, err := paths.Logs()
	require.NoError(t, err)
	logFile := filepath.Join(logsPath, "Quiver.log")

	shutdown := logger.Init(config.Logger{Enabled: true, Level: "debug"})
	t.Cleanup(func() {
		_ = shutdown()
		slog.SetDefault(prev)
		os.Remove(logFile)
	})

	slog.Info("probe")
	_, statErr := os.Stat(logFile)
	assert.NoError(t, statErr, "expected Quiver.log to be created in logs dir")
}

func TestInit_InvalidLevel_FallsBackToInfo(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	assert.NotPanics(t, func() {
		_ = logger.Init(config.Logger{Enabled: false, Level: "bogus"})
	})
}
```

- [ ] **Step 3: Run logger tests**

```bash
go test ./internal/core/logger/... -v
```

Expected: all 3 tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/core/logger/logger.go internal/core/logger/logger_test.go
git commit -m "feat(logger): use paths.Logs(), hardcode 5MB rotation, rename Init param to config.Logger"
```

---

### Task 4: Wire call sites — core.go and engine/container.go

**Files:**
- Modify: `internal/core/core.go`
- Modify: `internal/engine/container.go`

- [ ] **Step 1: Update `internal/core/core.go`**

Replace `config.GetWatcher()` with `config.GetLogger()`. The file becomes:

```go
package core

import (
	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/core/logger"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
)

type Core struct {
	metadata *metadata.Metadata
	config   *config.Config
}

func Init() *Core {
	_ = logger.Init(config.GetLogger())
	return &Core{
		metadata: metadata.Get(),
		config:   config.Get(),
	}
}

func (c *Core) GetMetadata() *metadata.Metadata {
	return c.metadata
}

func (c *Core) GetConfig() *config.Config {
	return c.config
}
```

- [ ] **Step 2: Update `internal/engine/container.go`**

Remove `const manifoldFetchTimeout`. Parse the timeout from config, falling back to 30s on parse failure. Replace the entire file:

```go
package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/core/paths"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

// Container holds all engine-layer dependencies.
type Container struct {
	Vault     vault.Vault
	Manifold  manifold.Manifold
	Wizard    wizard.Wizard
	Netbridge netbridge.Netbridge
	DepTree   deptree.DepTree
}

// Init constructs all engines and returns a ready-to-use Container.
func Init(ctx context.Context) (*Container, error) {
	eventsPath, err := paths.Events()
	if err != nil {
		return nil, fmt.Errorf("engine container: %w", err)
	}

	es, err := sqlite.NewEventStore(filepath.Join(eventsPath, "netbridge.db"))
	if err != nil {
		return nil, fmt.Errorf("engine container: %w", err)
	}

	nb, err := netbridge.New().WithEventStore(es).Build(ctx)
	if err != nil {
		return nil, fmt.Errorf("engine container: netbridge: %w", err)
	}

	wiz, err := wizard.New()
	if err != nil {
		return nil, fmt.Errorf("engine container: wizard: %w", err)
	}

	fetchTimeout, err := time.ParseDuration(config.GetManifold().FetchTimeout)
	if err != nil {
		fetchTimeout = 30 * time.Second
	}

	return &Container{
		Vault:     vault.New("", 0, domain.CurrentOS()),
		Manifold:  manifold.New(fetchTimeout),
		Wizard:    wiz,
		Netbridge: nb,
		DepTree:   deptree.New(),
	}, nil
}
```

- [ ] **Step 3: Build to verify**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Run full test suite**

```bash
go test ./... -count=1
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/core.go internal/engine/container.go
git commit -m "feat(engine): read manifold fetch_timeout from config; wire logger to config.GetLogger()"
```
