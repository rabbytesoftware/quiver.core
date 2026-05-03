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

func (m *module) Map(data []byte) (*domain.QuiverManifest, error) {
	var raw quiverV0
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal quiver@v0 YAML: %w", err)
	}
	return toAggregate(raw)
}

func toAggregate(raw quiverV0) (*domain.QuiverManifest, error) {
	arrows, err := toArrows(raw.Arrows)
	if err != nil {
		return nil, fmt.Errorf("invalid arrows: %w", err)
	}

	return &domain.QuiverManifest{
		Name:        raw.Metadata.Name,
		Version:     raw.Metadata.Version,
		Description: raw.Metadata.Description,
		URL:         raw.Metadata.URL,
		Maintainers: raw.Metadata.Maintainers,
		Tags:        raw.Metadata.Tags,
		Media: domain.QuiverMedia{
			Icon:   raw.Metadata.Media.Icon,
			Banner: raw.Metadata.Media.Banner,
		},
		Arrows: arrows,
	}, nil
}

func toArrows(entries []arrowEntryV0) ([]domain.QuiverArrow, error) {
	result := make([]domain.QuiverArrow, len(entries))
	for i, e := range entries {
		if e.Path != "" && e.Namespace != "" {
			return nil, fmt.Errorf("arrow entry at index %d has both path and namespace set", i)
		}
		if e.Path == "" && e.Namespace == "" {
			return nil, fmt.Errorf("arrow entry at index %d has neither path nor namespace set", i)
		}
		ns := e.Namespace
		if e.Path != "" {
			ns = e.Path // placeholder: Task 6 will derive the proper namespace
		}
		result[i] = domain.QuiverArrow{Namespace: domain.Namespace(ns)}
	}
	return result, nil
}
