package v1

import (
	_ "embed"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

//go:embed schema.json
var schemaJSON []byte

type QuiverV1Mapper struct{}

func NewMapper() *QuiverV1Mapper {
	return &QuiverV1Mapper{}
}

func (m *QuiverV1Mapper) Map(yamlData map[string]interface{}) (*domain.Quiver, error) {
	quiverModel := &domain.Quiver{}

	if metadata, ok := yamlData["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); ok {
			quiverModel.Manifest.Name = name
		}
		if desc, ok := metadata["description"].(string); ok {
			quiverModel.Manifest.Description = desc
		}
	}

	if err := validateQuiver(quiverModel); err != nil {
		return nil, fmt.Errorf("quiver validation failed: %w", err)
	}

	return quiverModel, nil
}

// GetSchema returns the JSON schema for Quiver v1 validation
func (m *QuiverV1Mapper) GetSchema() ([]byte, error) {
	return schemaJSON, nil
}

func validateQuiver(quiver *domain.Quiver) error {
	if quiver.Manifest.Name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if quiver.Manifest.Description == "" {
		return fmt.Errorf("description cannot be empty")
	}
	return nil
}
