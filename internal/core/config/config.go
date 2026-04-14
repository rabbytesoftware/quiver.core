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
