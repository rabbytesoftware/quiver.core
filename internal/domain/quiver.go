package domain

type Quiver struct {
	Namespace Namespace
	Manifest  QuiverManifest
	Removed   bool
}

type QuiverManifest struct {
	Name        string
	Description string
	URL         string
	Maintainers []string
	Tags        []string
	Media       QuiverMedia
	Arrows      []Namespace
}

type QuiverMedia struct {
	Icon   string
	Banner string
}
