package domain

type ReleaseAsset struct {
	Name   string `yaml:"name"   json:"name"`
	URL    string `yaml:"url"    json:"url"`
	Digest string `yaml:"digest" json:"digest,omitempty"`
}
