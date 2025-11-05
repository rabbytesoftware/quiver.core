package v1

import (
	"testing"
)

func TestArrowV1Mapper_Map(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{
		"manifest": "arrow@v1",
		"metadata": map[string]interface{}{
			"name":        "test-arrow",
			"description": "Test Arrow",
			"version":     "1.0.0",
			"license":     "MIT",
			"quiver_url":  "https://example.com",
			"credits": []interface{}{
				map[string]interface{}{
					"name":  "Test User",
					"email": "test@example.com",
					"url":   "https://example.com",
				},
			},
		},
		"requirements": map[string]interface{}{
			"cpu_cores":    float64(2),
			"ram_gb":       float64(4),
			"disk_gb":      float64(10),
			"network_mbps": float64(10),
			"system":       []interface{}{"linux/amd64"},
		},
		"netbridge": []interface{}{
			map[string]interface{}{
				"name":     "TEST_PORT",
				"protocol": "tcp",
			},
		},
		"variables": []interface{}{
			map[string]interface{}{
				"name":      "TEST_VAR",
				"default":   "value",
				"sensitive": false,
			},
		},
		"methods": map[string]interface{}{
			"linux": map[string]interface{}{
				"amd64": map[string]interface{}{
					"install": []interface{}{"echo install"},
				},
			},
		},
	}

	arrow, err := mapper.Map(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if arrow.Name != "test-arrow" {
		t.Errorf("got name %s, want test-arrow", arrow.Name)
	}
	if arrow.Version != "1.0.0" {
		t.Errorf("got version %s, want 1.0.0", arrow.Version)
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
