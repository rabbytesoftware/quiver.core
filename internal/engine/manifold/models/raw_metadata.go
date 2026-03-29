package models

import "github.com/rabbytesoftware/quiver/internal/domain"

// RawMetadata is the nested metadata block in an arrow@v0 YAML manifest.
// domain.ArrowManifest is flat, so this intermediate struct is required.
type RawMetadata struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Version     string            `yaml:"version"`
	License     string            `yaml:"license"`
	Quiver      string            `yaml:"quiver"`
	Media       domain.QuiverMedia `yaml:"media"`
	Credits     []domain.Credit   `yaml:"credits"`
	Maintainers []string          `yaml:"maintainers"`
	Tags        []string          `yaml:"tags"`
}
