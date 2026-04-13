# Quiver Home Organization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reorganize `~/.quiver` into a clean directory structure and centralize all path resolution in `metadata` with typed OS-aware getters.

**Architecture:** A new `OsValue[T]` type in the metadata package handles scalar-or-map YAML for OS-specific path overrides. All paths are declared in a top-level `paths:` section of `metadata.yaml` and exposed via `GetHomePath()`, `GetEventsPath()`, `GetStorePath()`, `GetNamespacesPath()`, `GetConfigPath()`. Every engine/adapter call site switches to these getters. The config package gains proper merge semantics.

**Tech Stack:** Go 1.23+, `gopkg.in/yaml.v3`, `github.com/rabbytesoftware/quiver` module

---

## File Map

| Action | File |
|---|---|
| **Create** | `internal/core/metadata/osvalue.go` |
| **Create** | `internal/core/metadata/osvalue_test.go` |
| **Modify** | `internal/core/metadata/metadata.yaml` |
| **Modify** | `internal/core/metadata/metadata.go` |
| **Modify** | `internal/core/metadata/metadata_test.go` |
| **Modify** | `internal/core/config/config.go` |
| **Modify** | `internal/core/config/config_test.go` |
| **Modify** | `internal/adapter/container.go` |
| **Modify** | `internal/internal.go` |
| **Modify** | `internal/engine/container.go` |
| **Modify** | `internal/engine/vault/store.go` |
| **Modify** | `internal/app/arrow/builder.go` |
| **Modify** | `internal/app/quiver/builder.go` |

---

### Task 1: `OsValue[T]` — new type in the metadata package

**Files:**
- Create: `internal/core/metadata/osvalue.go`
- Create: `internal/core/metadata/osvalue_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/core/metadata/osvalue_test.go
package metadata

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

func TestOsValue_UnmarshalYAML_Scalar(t *testing.T) {
	var v OsValue[string]
	require.NoError(t, yaml.Unmarshal([]byte(`"~/.quiver"`), &v))
	assert.Equal(t, "~/.quiver", v.Default)
	assert.Empty(t, v.OS)
}

func TestOsValue_UnmarshalYAML_Map(t *testing.T) {
	input := `
default: "~/.quiver"
windows: 'C:\Users\{{USER}}\Documents\.quiver'
`
	var v OsValue[string]
	require.NoError(t, yaml.Unmarshal([]byte(input), &v))
	assert.Equal(t, "~/.quiver", v.Default)
	assert.Equal(t, `C:\Users\{{USER}}\Documents\.quiver`, v.OS["windows"])
}

func TestOsValue_UnmarshalYAML_InvalidNode(t *testing.T) {
	var v OsValue[string]
	err := yaml.Unmarshal([]byte("- item1\n- item2"), &v)
	assert.Error(t, err)
}

func TestOsValue_Resolve_UsesOSOverride(t *testing.T) {
	v := OsValue[string]{
		Default: "default-val",
		OS:      map[string]string{runtime.GOOS: "os-val"},
	}
	assert.Equal(t, "os-val", v.Resolve())
}

func TestOsValue_Resolve_FallsBackToDefault(t *testing.T) {
	v := OsValue[string]{
		Default: "default-val",
		OS:      map[string]string{"other-os": "other-val"},
	}
	assert.Equal(t, "default-val", v.Resolve())
}

func TestOsValue_Resolve_EmptyOS(t *testing.T) {
	v := OsValue[string]{Default: "only-default"}
	assert.Equal(t, "only-default", v.Resolve())
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /Users/char2cs/.superset/worktrees/quiver.core/fix/quiver-home-organization
go test ./internal/core/metadata/... -run TestOsValue -v
```

Expected: compilation failure — `OsValue` undefined.

- [ ] **Step 3: Implement `OsValue[T]`**

```go
// internal/core/metadata/osvalue.go
package metadata

import (
	"fmt"
	"runtime"

	yaml "gopkg.in/yaml.v3"
)

// OsValue is a value that can be overridden per operating system.
// In YAML it accepts either a scalar (default only) or a map with a "default"
// key plus optional OS keys ("windows", "linux", "darwin").
type OsValue[T any] struct {
	Default T
	OS      map[string]T
}

// Resolve returns the OS-specific value for runtime.GOOS if present,
// otherwise returns Default.
func (o OsValue[T]) Resolve() T {
	if v, ok := o.OS[runtime.GOOS]; ok {
		return v
	}
	return o.Default
}

func (o *OsValue[T]) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		return value.Decode(&o.Default)
	}
	if value.Kind == yaml.MappingNode {
		var m map[string]T
		if err := value.Decode(&m); err != nil {
			return err
		}
		o.Default = m["default"]
		for k, v := range m {
			if k == "default" {
				continue
			}
			if o.OS == nil {
				o.OS = make(map[string]T)
			}
			o.OS[k] = v
		}
		return nil
	}
	return fmt.Errorf("osvalue: expected scalar or map, got node kind %v", value.Kind)
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/core/metadata/... -run TestOsValue -v
```

Expected: all 6 `TestOsValue_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/metadata/osvalue.go internal/core/metadata/osvalue_test.go
git commit -m "feat(metadata): add OsValue[T] — OS-aware scalar-or-map YAML type"
```

---

### Task 2: Update `metadata.yaml` and `metadata.go`

**Files:**
- Modify: `internal/core/metadata/metadata.yaml`
- Modify: `internal/core/metadata/metadata.go`
- Modify: `internal/core/metadata/metadata_test.go`

- [ ] **Step 1: Update `metadata.yaml`**

Replace the entire `variables:` block and add a top-level `paths:` section. The file should become:

```yaml
# ? Metadata is a collection of variables that
# ? are used throughout the project for:
# ?  - Describe things
# ?  - Set defaults variables (config relative paths, etc)
# ?  - Versioning, credits, maintainers, etc

version:
  number: 25.9.0
  codename: "Freeman"

metadata:
  name: Quiver
  description: The future of wizards and package managers.
  author: Rabbyte Software
  url: https://quiver.ar
  license: GPL-3.0
  copyright: Copyright 2025 Rabbyte Software & char2cs.net
  maintainers:
    - name: Mateo Urrutia
      email: me@char2cs.net
      url: https://char2cs.net
    - name: Agustin Gil
      email:
      url: https://github.com/AgustinGil21
    - name: Luca Lombardo
      email:
      url: https://github.com/Lucacux

paths:
  home:
    default: "~/.quiver"
    windows: 'C:\Users\{{USER}}\Documents\.quiver'
  events: "{{home}}/state/events"
  store: "{{home}}/state/store"
  namespaces: "{{home}}/namespaces"
  config: "{{home}}/config.yaml"

platforms:
  github.com:
    raw_url: "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}"
    default_branch: main
  gitlab.com:
    raw_url: "https://gitlab.com/{user}/{repo}/-/raw/{branch}/{file}"
    default_branch: main
  bitbucket.org:
    raw_url: "https://bitbucket.org/{user}/{repo}/raw/{branch}/{file}"
    default_branch: main
```

- [ ] **Step 2: Rewrite `metadata.go`**

Replace the entire file:

```go
package metadata

import (
	_ "embed"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	yaml "gopkg.in/yaml.v3"
)

var (
	//go:embed metadata.yaml
	metadataByte []byte
	metadata     *Metadata
	once         sync.Once
)

type Version struct {
	Number   string `yaml:"number"`
	Codename string `yaml:"codename"`
}

type Maintainer struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
	URL   string `yaml:"url"`
}

type MetadataInfo struct {
	Name        string       `yaml:"name"`
	Description string       `yaml:"description"`
	Author      string       `yaml:"author"`
	URL         string       `yaml:"url"`
	License     string       `yaml:"license"`
	Copyright   string       `yaml:"copyright"`
	Maintainers []Maintainer `yaml:"maintainers"`
}

type Paths struct {
	Home       OsValue[string] `yaml:"home"`
	Events     string          `yaml:"events"`
	Store      string          `yaml:"store"`
	Namespaces string          `yaml:"namespaces"`
	Config     string          `yaml:"config"`
}

type Platform struct {
	RawURL        string `yaml:"raw_url"`
	DefaultBranch string `yaml:"default_branch"`
}

type Platforms map[string]Platform

type Metadata struct {
	Version   Version      `yaml:"version"`
	Metadata  MetadataInfo `yaml:"metadata"`
	Paths     Paths        `yaml:"paths"`
	Platforms Platforms    `yaml:"platforms"`
}

func Get() *Metadata {
	once.Do(func() {
		metadata = &Metadata{}
		err := yaml.Unmarshal(metadataByte, metadata)
		if err != nil {
			metadata = defaultMetadata()
		}
	})
	return metadata
}

func GetVersion() string {
	return Get().Version.Number
}

func GetVersionCodename() string {
	return Get().Version.Codename
}

func GetName() string {
	return Get().Metadata.Name
}

func GetDescription() string {
	return Get().Metadata.Description
}

func GetAuthor() string {
	return Get().Metadata.Author
}

func GetURL() string {
	return Get().Metadata.URL
}

func GetLicense() string {
	return Get().Metadata.License
}

func GetCopyright() string {
	return Get().Metadata.Copyright
}

func GetMaintainers() []Maintainer {
	return Get().Metadata.Maintainers
}

func GetPlatforms() Platforms {
	return Get().Platforms
}

// GetHomePath returns the resolved, absolute Quiver home directory for the current OS.
func GetHomePath() string {
	return resolveHome()
}

// GetEventsPath returns the absolute path to the directory where event store
// databases are kept (~/.quiver/state/events on Unix).
func GetEventsPath() string {
	return resolvePath(Get().Paths.Events, resolveHome())
}

// GetStorePath returns the absolute path to the directory where catalog read
// model databases are kept (~/.quiver/state/store on Unix).
func GetStorePath() string {
	return resolvePath(Get().Paths.Store, resolveHome())
}

// GetNamespacesPath returns the absolute path to the namespaces directory,
// which is the working directory for installed arrows (~/.quiver/namespaces on Unix).
func GetNamespacesPath() string {
	return resolvePath(Get().Paths.Namespaces, resolveHome())
}

// GetConfigPath returns the absolute path to the user config file
// (~/.quiver/config.yaml on Unix).
func GetConfigPath() string {
	return resolvePath(Get().Paths.Config, resolveHome())
}

// resolveHome expands the OS-specific home template into an absolute path.
func resolveHome() string {
	raw := Get().Paths.Home.Resolve()
	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(raw, "{{USER}}", currentUsername())
	}
	if strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, raw[2:])
		}
	}
	return raw
}

// resolvePath replaces {{home}} in a path template with the resolved home.
func resolvePath(tmpl, home string) string {
	return strings.ReplaceAll(tmpl, "{{home}}", home)
}

func currentUsername() string {
	u, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return u.Username
}

func resetForTesting() {
	metadata = nil
	once = sync.Once{}
}

func defaultMetadata() *Metadata {
	return &Metadata{
		Version: Version{
			Number:   "25.9.0",
			Codename: "Freeman",
		},
		Metadata: MetadataInfo{
			Name:        "Quiver",
			Description: "The future of wizards and package managers.",
			Author:      "Rabbyte Software",
			URL:         "https://quiver.ar",
			License:     "GPL-3.0",
			Copyright:   "Copyright 2025 Rabbyte Software & char2cs.net",
			Maintainers: []Maintainer{
				{
					Name:  "Mateo Urrutia",
					Email: "me@char2cs.net",
					URL:   "https://char2cs.net",
				},
			},
		},
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
		},
		Platforms: Platforms{
			"github.com": {
				RawURL:        "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}",
				DefaultBranch: "main",
			},
			"gitlab.com": {
				RawURL:        "https://gitlab.com/{user}/{repo}/-/raw/{branch}/{file}",
				DefaultBranch: "main",
			},
			"bitbucket.org": {
				RawURL:        "https://bitbucket.org/{user}/{repo}/raw/{branch}/{file}",
				DefaultBranch: "main",
			},
		},
	}
}
```

- [ ] **Step 3: Replace `metadata_test.go`**

Replace the entire file with tests for the new surface. The old `GetQuiverHome`, `GetDefaultConfigPath`, `GetVariables`, `currentUsername` tests are deleted. New path getter tests are added:

```go
package metadata

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet_ReturnsSingleton(t *testing.T) {
	assert.Same(t, Get(), Get())
}

func TestGetVersion_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetVersion())
}

func TestGetVersionCodename_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetVersionCodename())
}

func TestGetName_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetName())
}

func TestGetDescription_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetDescription())
}

func TestGetAuthor_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetAuthor())
}

func TestGetURL_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetURL())
}

func TestGetLicense_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetLicense())
}

func TestGetCopyright_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetCopyright())
}

func TestMetadataStructure_AllFieldsPopulated(t *testing.T) {
	m := Get()
	assert.NotEmpty(t, m.Version.Number)
	assert.NotEmpty(t, m.Version.Codename)
	assert.NotEmpty(t, m.Metadata.Name)
	assert.NotEmpty(t, m.Metadata.Description)
	assert.NotEmpty(t, m.Metadata.Author)
	assert.NotEmpty(t, m.Metadata.URL)
	assert.NotEmpty(t, m.Metadata.License)
	assert.NotEmpty(t, m.Metadata.Copyright)
}

func TestDefaultMetadata_NonNil(t *testing.T) {
	require.NotNil(t, defaultMetadata())
}

func TestDefaultMetadata_PathsPopulated(t *testing.T) {
	d := defaultMetadata()
	assert.NotEmpty(t, d.Paths.Home.Default)
	assert.NotEmpty(t, d.Paths.Events)
	assert.NotEmpty(t, d.Paths.Store)
	assert.NotEmpty(t, d.Paths.Namespaces)
	assert.NotEmpty(t, d.Paths.Config)
}

func TestGetHomePath_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetHomePath())
}

func TestGetHomePath_EndsWithQuiver(t *testing.T) {
	assert.True(t, strings.HasSuffix(GetHomePath(), ".quiver"),
		"expected path to end in .quiver, got %q", GetHomePath())
}

func TestGetHomePath_Unix_AbsoluteUnderUserHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix test")
	}
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".quiver"), GetHomePath())
}

func TestGetEventsPath_ContainsHome(t *testing.T) {
	assert.True(t, strings.HasPrefix(GetEventsPath(), GetHomePath()))
}

func TestGetEventsPath_EndsWithStateEvents(t *testing.T) {
	assert.True(t, strings.HasSuffix(GetEventsPath(), filepath.Join("state", "events")))
}

func TestGetStorePath_ContainsHome(t *testing.T) {
	assert.True(t, strings.HasPrefix(GetStorePath(), GetHomePath()))
}

func TestGetStorePath_EndsWithStateStore(t *testing.T) {
	assert.True(t, strings.HasSuffix(GetStorePath(), filepath.Join("state", "store")))
}

func TestGetNamespacesPath_ContainsHome(t *testing.T) {
	assert.True(t, strings.HasPrefix(GetNamespacesPath(), GetHomePath()))
}

func TestGetNamespacesPath_EndsWithNamespaces(t *testing.T) {
	assert.True(t, strings.HasSuffix(GetNamespacesPath(), "namespaces"))
}

func TestGetConfigPath_ContainsHome(t *testing.T) {
	assert.True(t, strings.HasPrefix(GetConfigPath(), GetHomePath()))
}

func TestGetConfigPath_EndsWithConfigYaml(t *testing.T) {
	assert.True(t, strings.HasSuffix(GetConfigPath(), "config.yaml"))
}

func TestGetPlatforms_ReturnsKnownDomains(t *testing.T) {
	platforms := GetPlatforms()
	require.NotNil(t, platforms)
	for _, domain := range []string{"github.com", "gitlab.com", "bitbucket.org"} {
		assert.Contains(t, platforms, domain, "expected %q in platforms", domain)
	}
}

func TestGetPlatforms_GitHubRawURL(t *testing.T) {
	github := GetPlatforms()["github.com"]
	assert.Contains(t, github.RawURL, "raw.githubusercontent.com")
	assert.Equal(t, "main", github.DefaultBranch)
}

func TestMetadataConsistency(t *testing.T) {
	m := Get()
	assert.Equal(t, GetVersion(), m.Version.Number)
	assert.Equal(t, GetVersionCodename(), m.Version.Codename)
	assert.Equal(t, GetName(), m.Metadata.Name)
	assert.Equal(t, GetDescription(), m.Metadata.Description)
	assert.Equal(t, GetAuthor(), m.Metadata.Author)
	assert.Equal(t, GetURL(), m.Metadata.URL)
	assert.Equal(t, GetLicense(), m.Metadata.License)
	assert.Equal(t, GetCopyright(), m.Metadata.Copyright)
}
```

- [ ] **Step 4: Run metadata tests**

```bash
go test ./internal/core/metadata/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/metadata/metadata.yaml internal/core/metadata/metadata.go internal/core/metadata/metadata_test.go
git commit -m "feat(metadata): centralize path resolution — GetHomePath/EventsPath/StorePath/NamespacesPath/ConfigPath"
```

---

### Task 3: Update `config.go` — merge semantics, remove path ownership

**Files:**
- Modify: `internal/core/config/config.go`
- Modify: `internal/core/config/config_test.go`

- [ ] **Step 1: Rewrite `config.go`**

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

type Arrows struct {
	Repositories []string `yaml:"repositories"`
	InstallDir   string   `yaml:"install_dir"`
}

type API struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type Database struct {
	Path string `yaml:"path"`
}

type Watcher struct {
	Enabled  bool   `yaml:"enabled"`
	Level    string `yaml:"level"`
	Folder   string `yaml:"folder"`
	MaxSize  int    `yaml:"max_size"`
	MaxAge   int    `yaml:"max_age"`
	Compress bool   `yaml:"compress"`
}

type ConfigData struct {
	Netbridge Netbridge `yaml:"netbridge"`
	Arrows    Arrows    `yaml:"arrows"`
	API       API       `yaml:"api"`
	Database  Database  `yaml:"database"`
	Watcher   Watcher   `yaml:"watcher"`
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

func GetArrows() Arrows {
	return Get().Config.Arrows
}

func GetAPI() API {
	return Get().Config.API
}

func GetDatabase() Database {
	return Get().Config.Database
}

func GetWatcher() Watcher {
	return Get().Config.Watcher
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
			Arrows: Arrows{
				Repositories: []string{
					"https://raw.githubusercontent.com/rabbytesoftware/quiver.arrows/main",
				},
				InstallDir: "arrows",
			},
			API: API{
				Host: "0.0.0.0",
				Port: 40257,
			},
			Database: Database{
				Path: "db",
			},
			Watcher: Watcher{
				Enabled:  true,
				Level:    "info",
				Folder:   "logs",
				MaxSize:  100,
				MaxAge:   7,
				Compress: true,
			},
		},
	}
}

func resetForTesting() {
	config = nil
	once = sync.Once{}
}
```

- [ ] **Step 2: Update `config_test.go`**

Remove `TestGetConfigPath`, `TestConfigExists`, and all references to `GetConfigPath()` and `ConfigExists()`. Update `TestGet_WithValidConfigFile_ReadsFile` to use `metadata.GetConfigPath()`. Replace the entire file:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"

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
	assert.NotEmpty(t, cfg.Config.Database.Path)
	assert.NotEmpty(t, cfg.Config.Watcher.Level)
	assert.NotEmpty(t, cfg.Config.Watcher.Folder)
	assert.Positive(t, cfg.Config.Netbridge.EphemeralPortStart)
	assert.Positive(t, cfg.Config.Netbridge.EphemeralPortEnd)
}

func TestGetNetbridge_FieldsAccessible(t *testing.T) {
	nb := GetNetbridge()
	_ = nb.Enabled
	_ = nb.EphemeralPortStart
	_ = nb.EphemeralPortEnd
}

func TestGetArrows_FieldsAccessible(t *testing.T) {
	arrows := GetArrows()
	_ = arrows.Repositories
	_ = arrows.InstallDir
}

func TestGetAPI_ValidValues(t *testing.T) {
	api := GetAPI()
	assert.NotEmpty(t, api.Host)
	assert.Positive(t, api.Port)
}

func TestGetDatabase_ValidPath(t *testing.T) {
	assert.NotEmpty(t, GetDatabase().Path)
}

func TestGetWatcher_ValidValues(t *testing.T) {
	w := GetWatcher()
	assert.NotEmpty(t, w.Level)
	assert.NotEmpty(t, w.Folder)
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
	// Watcher should still have defaults since it wasn't in the partial override.
	assert.NotEmpty(t, cfg.Config.Watcher.Level)
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

- [ ] **Step 3: Run config tests**

```bash
go test ./internal/core/config/... -v
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/core/config/config.go internal/core/config/config_test.go
git commit -m "feat(config): merge semantics + path ownership moved to metadata"
```

---

### Task 4: Update all call sites

**Files:**
- Modify: `internal/adapter/container.go`
- Modify: `internal/internal.go`
- Modify: `internal/engine/container.go`
- Modify: `internal/engine/vault/store.go`
- Modify: `internal/app/arrow/builder.go`
- Modify: `internal/app/quiver/builder.go`

- [ ] **Step 1: Update `adapter/container.go`**

Remove the `home string` parameter. Use `metadata.GetEventsPath()` for all event stores. Ensure the directory exists before opening DBs.

```go
package adapter

import (
	"fmt"
	"os"
	"path/filepath"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
)

type Container struct {
	ArrowES   asynxModels.Store
	RuntimeES asynxModels.Store
	QuiverES  asynxModels.Store
}

func Init() (*Container, error) {
	eventsPath := metadata.GetEventsPath()
	if err := os.MkdirAll(eventsPath, 0750); err != nil {
		return nil, fmt.Errorf("adapter: create events dir: %w", err)
	}

	arrowES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "arrow.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: arrow event store: %w", err)
	}

	runtimeES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "runtime.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: runtime event store: %w", err)
	}

	quiverES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "quiver.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: quiver event store: %w", err)
	}

	return &Container{
		ArrowES:   arrowES,
		RuntimeES: runtimeES,
		QuiverES:  quiverES,
	}, nil
}
```

- [ ] **Step 2: Update `internal/internal.go`**

Remove the `metadata.GetQuiverHome()` argument from `adapter.Init()`:

```go
package internal

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/adapter"
	"github.com/rabbytesoftware/quiver/internal/api"
	"github.com/rabbytesoftware/quiver/internal/app"
	"github.com/rabbytesoftware/quiver/internal/engine"
)

type Container struct {
	Engines  *engine.Container
	Adapters *adapter.Container
	WsHub    *api.Hub
	App      *app.Container
	API      *api.Container
}

// Init wires all internal modules together: engine + adapter → app → api.
func Init(ctx context.Context) (*Container, error) {
	engines, err := engine.Init(ctx)
	if err != nil {
		return nil, fmt.Errorf("internal: engine: %w", err)
	}

	adapters, err := adapter.Init()
	if err != nil {
		return nil, fmt.Errorf("internal: adapter: %w", err)
	}

	wshub := api.NewHub()

	appContainer, err := app.Init(engines, adapters, wshub)
	if err != nil {
		return nil, fmt.Errorf("internal: app: %w", err)
	}

	apiContainer, err := api.Init(appContainer, wshub)
	if err != nil {
		return nil, fmt.Errorf("internal: api: %w", err)
	}

	return &Container{
		Engines:  engines,
		Adapters: adapters,
		WsHub:    wshub,
		App:      appContainer,
		API:      apiContainer,
	}, nil
}
```

- [ ] **Step 3: Update `engine/container.go`**

Delete `openEventStore`. Use `metadata.GetEventsPath()` directly. Ensure the events dir exists once:

```go
package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

const manifoldFetchTimeout = 30 * time.Second

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
	eventsPath := metadata.GetEventsPath()
	if err := os.MkdirAll(eventsPath, 0750); err != nil {
		return nil, fmt.Errorf("engine container: create events dir: %w", err)
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

	return &Container{
		Vault:     vault.New("", 0, domain.CurrentOS()),
		Manifold:  manifold.New(manifoldFetchTimeout),
		Wizard:    wiz,
		Netbridge: nb,
		DepTree:   deptree.New(),
	}, nil
}
```

- [ ] **Step 4: Update `engine/vault/store.go`**

Change the `basePath == ""` fallback to use `metadata.GetNamespacesPath()`. Remove the `"namespaces"` subdirectory appending inside `acquireNamespace` — `basePath` now IS the namespaces root:

```go
package vault

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type store struct {
	basePath  string
	ttl       time.Duration
	osVersion domain.OS
	clock     func() time.Time
	mu        sync.RWMutex
	locks     map[string]*sync.Mutex
}

func New(
	basePath string,
	ttl time.Duration,
	osVersion domain.OS,
) Vault {
	if basePath == "" {
		basePath = metadata.GetNamespacesPath()
	}
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	return &store{
		basePath:  basePath,
		ttl:       ttl,
		osVersion: osVersion,
		clock:     time.Now,
		mu:        sync.RWMutex{},
		locks:     make(map[string]*sync.Mutex),
	}
}

// namespaceLock returns the per-namespace mutex, creating it on first access.
func (s *store) namespaceLock(key string) *sync.Mutex {
	s.mu.RLock()
	if m, ok := s.locks[key]; ok {
		s.mu.RUnlock()
		return m
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.locks[key]; ok {
		return m
	}
	m := &sync.Mutex{}
	s.locks[key] = m
	return m
}

// acquireNamespace validates the namespace path and returns the per-namespace
// mutex (unlocked) and the resolved directory path.
func (s *store) acquireNamespace(ns domain.Namespace) (*sync.Mutex, string, error) {
	base := s.basePath
	resolved := filepath.Clean(filepath.Join(base, ns.String()))
	if !strings.HasPrefix(resolved, base+string(filepath.Separator)) {
		return nil, "", ErrInvalidNamespace
	}
	return s.namespaceLock(ns.String()), resolved, nil
}

func (s *store) GetArrow(
	ctx context.Context,
	namespace domain.Namespace,
) (*VaultEntry, string, error) {
	if err := namespace.Validate(); err != nil {
		return nil, "", ErrInvalidNamespace
	}
	return getArrow(s, namespace)
}

func (s *store) GetQuiver(
	ctx context.Context,
	namespace domain.Namespace,
) (*QuiverVaultEntry, string, error) {
	if err := namespace.Validate(); err != nil {
		return nil, "", ErrInvalidNamespace
	}
	return getQuiver(s, namespace)
}

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

func (s *store) DeleteArrow(
	ctx context.Context,
	namespace domain.Namespace,
) error {
	if err := namespace.Validate(); err != nil {
		return ErrInvalidNamespace
	}
	return deleteArrow(s, namespace)
}

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

- [ ] **Step 5: Update `app/arrow/builder.go`**

Replace `metadata.GetQuiverHome() + "/arrows.db"` with `filepath.Join(metadata.GetStorePath(), "arrows.db")`. Add `"path/filepath"` import if not present:

```go
store, storeErr := arrowstore.NewArrowCatalog(filepath.Join(metadata.GetStorePath(), "arrows.db"))
```

Also add `os.MkdirAll` for the store dir before opening the catalog:

```go
if mkErr := os.MkdirAll(metadata.GetStorePath(), 0750); mkErr != nil {
    return nil, fmt.Errorf("arrow builder: create store dir: %w", mkErr)
}
store, storeErr := arrowstore.NewArrowCatalog(filepath.Join(metadata.GetStorePath(), "arrows.db"))
```

- [ ] **Step 6: Update `app/quiver/builder.go`**

Same pattern as arrow builder:

```go
if mkErr := os.MkdirAll(metadata.GetStorePath(), 0750); mkErr != nil {
    return nil, fmt.Errorf("quiver builder: create store dir: %w", mkErr)
}
store, storeErr := quiverstore.NewQuiverCatalog(filepath.Join(metadata.GetStorePath(), "quivers.db"))
```

- [ ] **Step 7: Build to verify no compilation errors**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 8: Run full test suite**

```bash
go test ./... -count=1
```

Expected: all tests PASS. Vault tests still pass because they inject a `t.TempDir()` as `basePath` — which now acts as the namespaces root directly (no `"namespaces"` subdir appended), so the path structure is different from production but semantically identical for testing.

- [ ] **Step 9: Commit**

```bash
git add internal/adapter/container.go internal/internal.go internal/engine/container.go internal/engine/vault/store.go internal/app/arrow/builder.go internal/app/quiver/builder.go
git commit -m "feat(paths): wire all engines and adapters to centralized metadata path getters"
```
