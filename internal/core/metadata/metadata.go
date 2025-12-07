package metadata

import (
	_ "embed"
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

type Variables struct {
	DefaultConfigPath string `yaml:"DEFAULT_CONFIG_PATH"`
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
		},
	}
}
