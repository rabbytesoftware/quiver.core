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
