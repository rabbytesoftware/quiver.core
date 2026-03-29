package v0

import (
	_ "embed"

	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"gopkg.in/yaml.v3"
)

//go:embed schema.json
var schemaJSON []byte

// ArrowV0Mapper validates and unmarshals arrow@v0 YAML into a RawArrow.
type ArrowV0Mapper struct{}

// NewMapper returns a new ArrowV0Mapper.
func NewMapper() *ArrowV0Mapper {
	return &ArrowV0Mapper{}
}

// Map unmarshals a YAML data map into a *models.RawArrow by re-marshalling
// the map back to YAML bytes and decoding into the typed struct.
func (m *ArrowV0Mapper) Map(
	data []byte,
) (*models.RawArrow, error) {
	raw := &models.RawArrow{}
	if err := yaml.Unmarshal(data, raw); err != nil {
		return nil, err
	}

	return raw, nil
}

// GetSchema returns the embedded JSON schema for arrow@v0 validation.
func (m *ArrowV0Mapper) GetSchema() ([]byte, error) {
	return schemaJSON, nil
}
