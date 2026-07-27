package metadata

import (
	_ "embed"
	"path/filepath"
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
	Logs       string          `yaml:"logs"`
	Vault      string          `yaml:"vault"`
}

// Kind names the host a platform is, which is what decides how its answers are
// read: the shape of its search response and of the redirect its release
// permalink hands back. Every platform declares one, because a host Quiver has
// no code for is a host it cannot read.
const (
	KindGitHub    = "github"
	KindGitLab    = "gitlab"
	KindBitbucket = "bitbucket"
)

// Platform describes how one git host serves raw files and, optionally, how it
// answers repository searches. DefaultBranches is tried in order when a
// namespace carries no explicit ref.
//
// LatestReleaseURL is a page that redirects to the host's latest stable
// release. It is followed for its Location header only and costs no API quota,
// so it is an optimisation over listing tags — never a requirement. A platform
// with no such page leaves it empty.
//
// SearchURL is optional in the same way: a platform without one still serves
// manifests, it just answers no query.
type Platform struct {
	Kind             string   `yaml:"kind"`
	RawURL           string   `yaml:"raw_url"`
	DefaultBranches  []string `yaml:"default_branches"`
	LatestReleaseURL string   `yaml:"latest_release_url"`
	SearchURL        string   `yaml:"search_url"`
}

type Platforms map[string]Platform

// Discovery holds the markers a repository must carry to be considered an
// arrow candidate. Topics is a list so new markers can be added without a
// schema change.
type Discovery struct {
	Topics []string `yaml:"topics"`
}

type Metadata struct {
	Version   Version      `yaml:"version"`
	Metadata  MetadataInfo `yaml:"metadata"`
	Paths     Paths        `yaml:"paths"`
	Platforms Platforms    `yaml:"platforms"`
	Discovery Discovery    `yaml:"discovery"`
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

// GetDiscovery returns the discovery markers used to recognise arrow
// candidates on a git host.
func GetDiscovery() Discovery {
	return Get().Discovery
}

func GetHomePath() string {
	return resolveHome()
}

func GetEventsPath() string {
	return resolvePath(Get().Paths.Events, resolveHome())
}

func GetEventsPathAt(homeDir string) string {
	return resolvePath(Get().Paths.Events, homeDir)
}

func GetStorePath() string {
	return resolvePath(Get().Paths.Store, resolveHome())
}

func GetStorePathAt(homeDir string) string {
	return resolvePath(Get().Paths.Store, homeDir)
}

func GetNamespacesPath() string {
	return resolvePath(Get().Paths.Namespaces, resolveHome())
}

func GetNamespacesPathAt(homeDir string) string {
	return resolvePath(Get().Paths.Namespaces, homeDir)
}

func GetVaultPath() string {
	return resolvePath(Get().Paths.Vault, resolveHome())
}

func GetVaultPathAt(homeDir string) string {
	return resolvePath(Get().Paths.Vault, homeDir)
}

func GetConfigPath() string {
	return resolvePath(Get().Paths.Config, resolveHome())
}

func GetLogsPath() string {
	return resolvePath(Get().Paths.Logs, resolveHome())
}

// resolvePath replaces {{home}} in a path template with the resolved home,
// then normalizes separators to the OS-native form.
func resolvePath(tmpl, home string) string {
	return filepath.FromSlash(strings.ReplaceAll(tmpl, "{{home}}", home))
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
			Logs:       "{{home}}/logs",
			Vault:      "{{home}}/vault",
		},
		Platforms: Platforms{
			"github.com": {
				Kind:             KindGitHub,
				RawURL:           "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}",
				DefaultBranches:  []string{"main", "master"},
				LatestReleaseURL: "https://github.com/{user}/{repo}/releases/latest",
				SearchURL:        "https://api.github.com/search/repositories?q={query}",
			},
			"gitlab.com": {
				Kind:             KindGitLab,
				RawURL:           "https://gitlab.com/{user}/{repo}/-/raw/{branch}/{file}",
				DefaultBranches:  []string{"main", "master"},
				LatestReleaseURL: "https://gitlab.com/{user}/{repo}/-/releases/permalink/latest",
				SearchURL:        "https://gitlab.com/api/v4/projects?search={query}&topic={topic}",
			},
			"bitbucket.org": {
				Kind:            KindBitbucket,
				RawURL:          "https://bitbucket.org/{user}/{repo}/raw/{branch}/{file}",
				DefaultBranches: []string{"main", "master"},
			},
		},
		Discovery: Discovery{
			Topics: []string{"quiver-arrow"},
		},
	}
}
