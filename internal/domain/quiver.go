package domain

type Quiver struct {
	Namespace Namespace      `json:"namespace"`
	Manifest  QuiverManifest `json:"manifest"`
	Removed   bool           `json:"removed"`
}

type QuiverManifest struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	URL         string      `json:"url"`
	Maintainers []string    `json:"maintainers"`
	Tags        []string    `json:"tags"`
	Media       QuiverMedia `json:"media"`
	Arrows      []Namespace `json:"arrows"`
}

type QuiverMedia struct {
	Icon   string `json:"icon"`
	Banner string `json:"banner"`
}
