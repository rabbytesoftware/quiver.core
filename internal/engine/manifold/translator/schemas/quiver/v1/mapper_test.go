package v1

import (
	"testing"
)

func TestQuiverV1Mapper_Map(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{
		"manifest": "quiver@v1",
		"metadata": map[string]interface{}{
			"name":        "test-quiver",
			"description": "Test Quiver",
			"version":     "1.0.0",
		},
	}

	quiver, err := mapper.Map(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if quiver.Manifest.Name != "test-quiver" {
		t.Errorf("got name %s, want test-quiver", quiver.Manifest.Name)
	}
}

func TestGetSchema(t *testing.T) {
	mapper := NewMapper()
	schema, err := mapper.GetSchema()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(schema) == 0 {
		t.Error("schema is empty")
	}
}

func TestQuiverV1Mapper_Map_MissingMetadata(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{
		"manifest": "quiver@v1",
	}

	_, err := mapper.Map(yamlData)
	if err == nil {
		t.Error("Map() should return error for missing metadata")
	}
}

func TestQuiverV1Mapper_Map_WrongMetadataType(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{
		"manifest": "quiver@v1",
		"metadata": "not-a-map",
	}

	_, err := mapper.Map(yamlData)
	if err == nil {
		t.Error("Map() should return error for wrong metadata type (no name/description)")
	}
}

func TestQuiverV1Mapper_Map_WrongNameType(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{
		"manifest": "quiver@v1",
		"metadata": map[string]interface{}{
			"name":    123,
			"version": "1.0.0",
		},
	}

	_, err := mapper.Map(yamlData)
	if err == nil {
		t.Error("Map() should return error when name is non-string (empty after extraction)")
	}
}

func TestQuiverV1Mapper_Map_WrongVersionType(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{
		"manifest": "quiver@v1",
		"metadata": map[string]interface{}{
			"name":        "test-quiver",
			"description": "A test quiver",
			"version":     123,
		},
	}

	quiver, err := mapper.Map(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if quiver.Manifest.Name != "test-quiver" {
		t.Errorf("got name %s, want test-quiver", quiver.Manifest.Name)
	}
}

func TestQuiverV1Mapper_Map_EmptyData(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{}

	_, err := mapper.Map(yamlData)
	if err == nil {
		t.Error("Map() should return error for empty data (no name/description)")
	}
}

func TestNewMapper(t *testing.T) {
	mapper := NewMapper()
	if mapper == nil {
		t.Fatal("NewMapper() returned nil")
	}
}
