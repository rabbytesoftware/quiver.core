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
