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

// QuiverHome holds the platform-specific default paths for the Quiver home directory.
type QuiverHome struct {
	WindowsHome string `yaml:"windows_home"`
	UnixHome    string `yaml:"unix_home"`
}

type Variables struct {
	DefaultConfigPath string     `yaml:"DEFAULT_CONFIG_PATH"`
	QuiverHome        QuiverHome `yaml:"QUIVER_HOME"`
}

type Metadata struct {
	Version   Version      `yaml:"version"`
	Metadata  MetadataInfo `yaml:"metadata"`
	Variables Variables    `yaml:"variables"`
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

func GetVariables() Variables {
	return Get().Variables
}

func GetDefaultConfigPath() string {
	return Get().Variables.DefaultConfigPath
}

// GetQuiverHome returns the platform-specific Quiver home directory path,
// resolving the current user's home directory and username at call time.
func GetQuiverHome() string {
	vars := Get().Variables.QuiverHome

	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(vars.WindowsHome, "{{USER}}", currentUsername())
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return vars.UnixHome
	}
	if strings.HasPrefix(vars.UnixHome, "~/") {
		return filepath.Join(home, vars.UnixHome[2:])
	}
	return vars.UnixHome
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
		Variables: Variables{
			DefaultConfigPath: "./config.yaml",
			QuiverHome: QuiverHome{
				WindowsHome: `C:\Users\{{USER}}\Documents\.quiver`,
				UnixHome:    "~/.quiver",
			},
		},
	}
}
