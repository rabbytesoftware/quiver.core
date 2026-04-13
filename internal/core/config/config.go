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
