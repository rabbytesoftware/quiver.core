package config

import (
	"context"
	_ "embed"
	"log/slog"
	"path/filepath"
	"sync"

	yaml "gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
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
}

type Logger struct {
	Enabled bool   `yaml:"enabled"`
	Level   string `yaml:"level"`
}

type Manifold struct {
	FetchTimeout string `yaml:"fetch_timeout"`
}

type Vault struct {
	SweepInterval string `yaml:"sweep_interval"`
	TTL           string `yaml:"ttl"`
	IndexTTL      string `yaml:"index_ttl"`
}

// Search configures online discovery. Quiver authenticates to no git host:
// discovery is anonymous, so it knows nothing about credentials and asks the
// user for none.
type Search struct {
	PerProviderLimit int    `yaml:"per_provider_limit"`
	FetchConcurrency int    `yaml:"fetch_concurrency"`
	ProviderTimeout  string `yaml:"provider_timeout"`
}

type ArrowAutoRetry struct {
	Enabled bool `yaml:"enabled"`
	Retries int  `yaml:"retries"`
}

type Arrows struct {
	AutoRetry ArrowAutoRetry `yaml:"auto_retry"`
}

type ConfigData struct {
	Netbridge Netbridge `yaml:"netbridge"`
	API       API       `yaml:"api"`
	Logger    Logger    `yaml:"logger"`
	Manifold  Manifold  `yaml:"manifold"`
	Vault     Vault     `yaml:"vault"`
	Arrows    Arrows    `yaml:"arrows"`
	Search    Search    `yaml:"search"`
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
			return
		}

		for _, fe := range Sanitize(&config.Config) {
			slog.Warn("config: invalid value, using default", "key", fe.Key, "reason", fe.Message)
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

func GetVault() Vault {
	return Get().Config.Vault
}

func GetArrows() Arrows {
	return Get().Config.Arrows
}

func GetSearch() Search {
	return Get().Config.Search
}

func getDefaultConfig() *Config {
	cfg := &Config{}
	if err := yaml.Unmarshal(defaultConfigByte, cfg); err != nil {
		// The embedded default.yaml is baked in at build time. A parse failure
		// means the binary itself is corrupt — there is no safe fallback.
		panic("config: failed to parse embedded default.yaml: " + err.Error())
	}
	return cfg
}

func resetForTesting() {
	config = nil
	once = sync.Once{}
}
