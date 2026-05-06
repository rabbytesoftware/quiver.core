package v0

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

//go:embed schema.json
var schemaJSON []byte

var Default = &module{}

type module struct{}

func (m *module) Version() string { return "v0" }

func (m *module) GetSchema() ([]byte, error) { return schemaJSON, nil }

func (m *module) Map(data []byte) (*domain.Collection, []domain.CollectionArrowEntry, error) {
	var raw quiverV0
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal quiver@v0 YAML: %w", err)
	}
	return toModule(raw)
}

func toModule(raw quiverV0) (*domain.Collection, []domain.CollectionArrowEntry, error) {
	entries := make([]domain.CollectionArrowEntry, len(raw.Arrows))
	for i, a := range raw.Arrows {
		entries[i] = domain.CollectionArrowEntry{Path: a.Path, Namespace: a.Namespace}
	}
	quiver := &domain.Collection{
		Meta: domain.CollectionMeta{
			Name:        raw.Metadata.Name,
			Version:     raw.Metadata.Version,
			Description: raw.Metadata.Description,
			URL:         raw.Metadata.URL,
			Maintainers: raw.Metadata.Maintainers,
			Tags:        raw.Metadata.Tags,
			Media: domain.CollectionMedia{
				Icon:   raw.Metadata.Media.Icon,
				Banner: raw.Metadata.Media.Banner,
			},
		},
		// Namespace, FollowedAt, FailedArrows, Arrows filled in by caller
	}
	return quiver, entries, nil
}
