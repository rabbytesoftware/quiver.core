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

	if quiver.Name != "test-quiver" {
		t.Errorf("got name %s, want test-quiver", quiver.Name)
	}
	if quiver.Version != "1.0.0" {
		t.Errorf("got version %s, want 1.0.0", quiver.Version)
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

	quiver, err := mapper.Map(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if quiver.Name != "" {
		t.Errorf("got name %s, want empty", quiver.Name)
	}
	if quiver.Version != "" {
		t.Errorf("got version %s, want empty", quiver.Version)
	}
}

func TestQuiverV1Mapper_Map_WrongMetadataType(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{
		"manifest": "quiver@v1",
		"metadata": "not-a-map",
	}

	quiver, err := mapper.Map(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if quiver.Name != "" {
		t.Errorf("got name %s, want empty", quiver.Name)
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

	quiver, err := mapper.Map(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if quiver.Name != "" {
		t.Errorf("got name %s, want empty", quiver.Name)
	}
}

func TestQuiverV1Mapper_Map_WrongVersionType(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{
		"manifest": "quiver@v1",
		"metadata": map[string]interface{}{
			"name":    "test-quiver",
			"version": 123,
		},
	}

	quiver, err := mapper.Map(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if quiver.Version != "" {
		t.Errorf("got version %s, want empty", quiver.Version)
	}
}

func TestQuiverV1Mapper_Map_EmptyData(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{}

	quiver, err := mapper.Map(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if quiver == nil {
		t.Fatal("got nil quiver")
	}
}

func TestNewMapper(t *testing.T) {
	mapper := NewMapper()
	if mapper == nil {
		t.Fatal("NewMapper() returned nil")
	}
}
