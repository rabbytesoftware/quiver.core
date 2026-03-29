package models

import "github.com/rabbytesoftware/quiver/internal/domain"

// RawQuiver is the top-level YAML structure for a quiver@v0 manifest.
// Fields are flat (no nested metadata block), unlike domain.QuiverManifest
// which has no schema field. Uses domain.QuiverMedia directly since its
// field names match the YAML keys.
type RawQuiver struct {
	Schema      string             `yaml:"schema"`
	Name        string             `yaml:"name"`
	Description string             `yaml:"description"`
	URL         string             `yaml:"url"`
	Maintainers []string           `yaml:"maintainers"`
	Tags        []string           `yaml:"tags"`
	Media       domain.QuiverMedia `yaml:"media"`
	Arrows      []string           `yaml:"arrows"`
}
