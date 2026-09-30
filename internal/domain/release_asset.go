package domain

type ReleaseAsset struct {
	Name   string `yaml:"name"            json:"name"`
	Label  string `yaml:"label,omitempty" json:"label,omitempty"`
	URL    string `yaml:"url"             json:"url"`
	Digest string `yaml:"digest"          json:"digest,omitempty"`
}
