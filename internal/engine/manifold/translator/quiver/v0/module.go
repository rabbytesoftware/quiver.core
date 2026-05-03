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
		Name:        raw.Name,
		Description: raw.Description,
		URL:         raw.URL,
		Maintainers: raw.Maintainers,
		Tags:        raw.Tags,
		Media: domain.QuiverMedia{
			Icon:   raw.Media.Icon,
			Banner: raw.Media.Banner,
		},
		Arrows: arrows,
	}, nil
}

func toArrows(arrows []string) ([]domain.QuiverArrow, error) {
	result := make([]domain.QuiverArrow, len(arrows))
	for i, a := range arrows {
		result[i] = domain.QuiverArrow{Namespace: domain.Namespace(a)}
	}
	return result, nil
}
