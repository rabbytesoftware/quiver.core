package domain

type Quiver struct {
	Namespace Namespace      `yaml:"namespace" json:"namespace"`
	Manifest  QuiverManifest `yaml:"manifest"  json:"manifest"`
	Removed   bool           `yaml:"removed"   json:"removed"`
}

type QuiverManifest struct {
	Name        string      `yaml:"name"        json:"name"`
	Description string      `yaml:"description" json:"description"`
	URL         string      `yaml:"url"         json:"url"`
	Maintainers []string    `yaml:"maintainers" json:"maintainers"`
	Tags        []string    `yaml:"tags"        json:"tags"`
	Media       QuiverMedia `yaml:"media"       json:"media"`
	Arrows      []Namespace `yaml:"arrows"      json:"arrows"`
}

type QuiverMedia struct {
	Icon   string `yaml:"icon"   json:"icon"`
	Banner string `yaml:"banner" json:"banner"`
}
