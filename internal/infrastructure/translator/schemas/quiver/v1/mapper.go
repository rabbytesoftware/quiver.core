package v1

import (
	_ "embed"

	"github.com/rabbytesoftware/quiver/internal/models/quiver"
)

//go:embed schema.json
var schemaJSON []byte

type QuiverV1Mapper struct{}

func NewMapper() *QuiverV1Mapper {
	return &QuiverV1Mapper{}
}

func (m *QuiverV1Mapper) Map(yamlData map[string]interface{}) (*quiver.Quiver, error) {
	quiverModel := &quiver.Quiver{}

	if metadata, ok := yamlData["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); ok {
			quiverModel.Name = name
		}
		if version, ok := metadata["version"].(string); ok {
			quiverModel.Version = version
		}
	}

	return quiverModel, nil
}

// GetSchema returns the JSON schema for Quiver v1 validation
func (m *QuiverV1Mapper) GetSchema() ([]byte, error) {
	return schemaJSON, nil
}
