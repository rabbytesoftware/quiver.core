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
	Enabled            bool `yaml:"enabled"              json:"enabled"`
	EphemeralPortStart int  `yaml:"ephemeral_port_start" json:"ephemeral_port_start" validate:"min=1,max=65535"`
	EphemeralPortEnd   int  `yaml:"ephemeral_port_end"   json:"ephemeral_port_end"   validate:"min=1,max=65535"`
}

type API struct {
	Host string `yaml:"host" json:"host" validate:"quiverhost"`
}

type Logger struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Level   string `yaml:"level"   json:"level"   validate:"loglevel"`
}

type Manifold struct {
	FetchTimeout string `yaml:"fetch_timeout" json:"fetch_timeout" validate:"duration"`
}

type Vault struct {
	SweepInterval string `yaml:"sweep_interval" json:"sweep_interval" validate:"duration"`
	TTL           string `yaml:"ttl"            json:"ttl"            validate:"duration"`
	IndexTTL      string `yaml:"index_ttl"      json:"index_ttl"      validate:"duration"`
}

// Search configures online discovery. Quiver authenticates to no git host:
// discovery is anonymous, so it knows nothing about credentials and asks the
// user for none.
type Search struct {
	PerProviderLimit int    `yaml:"per_provider_limit" json:"per_provider_limit" validate:"min=1"`
	FetchConcurrency int    `yaml:"fetch_concurrency"  json:"fetch_concurrency"  validate:"min=1"`
	ProviderTimeout  string `yaml:"provider_timeout"   json:"provider_timeout"   validate:"duration"`
}

type ArrowAutoRetry struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	Retries int  `yaml:"retries" json:"retries" validate:"min=0"`
}

type Arrows struct {
	AutoRetry ArrowAutoRetry `yaml:"auto_retry" json:"auto_retry"`
}

type ConfigData struct {
	Netbridge Netbridge `yaml:"netbridge" json:"netbridge"`
	API       API       `yaml:"api"       json:"api"`
	Logger    Logger    `yaml:"logger"    json:"logger"`
	Manifold  Manifold  `yaml:"manifold"  json:"manifold"`
	Vault     Vault     `yaml:"vault"     json:"vault"`
	Arrows    Arrows    `yaml:"arrows"    json:"arrows"`
	Search    Search    `yaml:"search"    json:"search"`
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
