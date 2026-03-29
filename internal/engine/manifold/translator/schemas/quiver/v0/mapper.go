package v0

import (
	_ "embed"

	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"gopkg.in/yaml.v3"
)

//go:embed schema.json
var schemaJSON []byte

// QuiverV0Mapper validates and unmarshals quiver@v0 YAML into a RawQuiver.
type QuiverV0Mapper struct{}

// NewMapper returns a new QuiverV0Mapper.
func NewMapper() *QuiverV0Mapper {
	return &QuiverV0Mapper{}
}

// Map unmarshals YAML bytes into a *models.RawQuiver.
func (m *QuiverV0Mapper) Map(
	data []byte,
) (*models.RawQuiver, error) {
	raw := &models.RawQuiver{}
	if err := yaml.Unmarshal(data, raw); err != nil {
		return nil, err
	}

	return raw, nil
}

// GetSchema returns the embedded JSON schema for quiver@v0 validation.
func (m *QuiverV0Mapper) GetSchema() ([]byte, error) {
	return schemaJSON, nil
}
