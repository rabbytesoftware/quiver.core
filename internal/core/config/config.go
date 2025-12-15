package config

import (
	"context"
	_ "embed"
	"errors"
	"io/fs"
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
	Enabled      bool   `yaml:"enabled"`
	AllowedPorts string `yaml:"allowed_ports"`
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

type Cache struct {
	Enabled         bool   `yaml:"enabled"`
	DefaultTTL      string `yaml:"default_ttl"`
	CleanupInterval string `yaml:"cleanup_interval"`
}

type ConfigData struct {
	Netbridge Netbridge `yaml:"netbridge"`
	Arrows    Arrows    `yaml:"arrows"`
	API       API       `yaml:"api"`
	Database  Database  `yaml:"database"`
	Watcher   Watcher   `yaml:"watcher"`
	Cache     Cache     `yaml:"cache"`
}

type Config struct {
	Config ConfigData `yaml:"config"`
}

func Get() *Config {
	once.Do(func() {
		configPath := filepath.Clean(metadata.GetDefaultConfigPath())
		configBytes, err := fns.Read(context.Background(), configPath)
		if err != nil {
			config = getDefaultConfig()
			return
		}

		config = &Config{}
		err = yaml.Unmarshal(configBytes, config)
		if err != nil {
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

func GetCache() Cache {
	return Get().Config.Cache
}

func GetConfigPath() string {
	return metadata.GetDefaultConfigPath()
}

func ConfigExists() bool {
	configPath := GetConfigPath()
	_, _, _, err := fns.GetInfo(context.Background(), configPath) // Could also use fns.Exists()
	return !errors.Is(err, fs.ErrNotExist)                        // Removed os.ErrNotExist for compatibility with fns package
}

func getDefaultConfig() *Config {
	config = &Config{}
	err := yaml.Unmarshal(defaultConfigByte, config)
	if err == nil {
		return config
	}

	return &Config{
		Config: ConfigData{
			Netbridge: Netbridge{
				Enabled:      true,
				AllowedPorts: "40128-40256",
			},
			Arrows: Arrows{
				Repositories: []string{
					"./pkgs",
				},
				InstallDir: "./arrows",
			},
			API: API{
				Host: "0.0.0.0",
				Port: 40257,
			},
			Database: Database{
				Path: "./.db",
			},
			Watcher: Watcher{
				Enabled:  true,
				Level:    "info",
				Folder:   "./logs",
				MaxSize:  100,
				MaxAge:   7,
				Compress: true,
			},
			Cache: Cache{
				Enabled:         false,
				DefaultTTL:      "5m",
				CleanupInterval: "1m",
			},
		},
	}
}

func resetForTesting() {
	config = nil
	once = sync.Once{}
}
