package v1

import (
	"testing"
)

func TestArrowV1Mapper_Map(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{
		"schema": "arrow@v1",
		"metadata": map[string]interface{}{
			"name":        "test-arrow",
			"description": "Test Arrow",
			"version":     "1.0.0",
			"license":     "MIT",
			"quiver":      "github.com/test/test",
			"media": map[string]interface{}{
				"icon":   "https://example.com/icon.png",
				"banner": "https://example.com/banner.png",
			},
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
			"network_mbps": float64(100),
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
				"default":   "value1",
				"sensitive": false,
				"values":    []interface{}{"value1", "value2"},
				"min":       float64(1),
				"max":       float64(10),
			},
		},
		"wizards": []interface{}{
			map[string]interface{}{
				"platforms":    []interface{}{"linux/amd64", "linux/arm64"},
				"dependencies": []interface{}{"dep1", "dep2"},
				"workdir":      "/app",
				"methods": []interface{}{
					map[string]interface{}{
						"method": "install",
						"actions": []interface{}{
							map[string]interface{}{
								"run":  "echo install",
								"name": "Installing",
							},
						},
					},
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
	if arrow.QuiverURL != "github.com/test/test" {
		t.Errorf("got quiver URL %s, want github.com/test/test", arrow.QuiverURL)
	}
	if arrow.IconURL != "https://example.com/icon.png" {
		t.Errorf("got icon URL %s, want https://example.com/icon.png", arrow.IconURL)
	}
	if arrow.Requirements.NetworkMbps != 100 {
		t.Errorf("got network %d, want 100", arrow.Requirements.NetworkMbps)
	}
	if len(arrow.Variables) > 0 {
		if len(arrow.Variables[0].Values) != 2 {
			t.Errorf("got %d values, want 2", len(arrow.Variables[0].Values))
		}
	}
	if len(arrow.Methods) == 0 {
		t.Error("expected at least one method")
	} else {
		if arrow.Methods[0].MethodName != "install" {
			t.Errorf("got method name %s, want install", arrow.Methods[0].MethodName)
		}
		if len(arrow.Methods[0].Platforms) != 2 {
			t.Errorf("got %d platforms, want 2", len(arrow.Methods[0].Platforms))
		}
		if len(arrow.Methods[0].Actions) == 0 {
			t.Error("expected at least one action")
		}
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

func TestArrowV1Mapper_Map_MinimalData(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{
		"schema": "arrow@v1",
		"metadata": map[string]interface{}{
			"name":    "minimal-arrow",
			"version": "1.0.0",
		},
		"requirements": map[string]interface{}{
			"cpu_cores":    float64(1),
			"ram_gb":       float64(1),
			"disk_gb":      float64(1),
			"network_mbps": float64(1),
			"system":       []interface{}{"linux/amd64"},
		},
	}

	arrow, err := mapper.Map(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if arrow.Name != "minimal-arrow" {
		t.Errorf("got name %s, want minimal-arrow", arrow.Name)
	}
}

func TestArrowV1Mapper_Map_InvalidData(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{
		"schema": "arrow@v1",
		"metadata": map[string]interface{}{
			"name":    "",
			"version": "",
		},
	}

	_, err := mapper.Map(yamlData)
	if err == nil {
		t.Error("Map() should return error for invalid data")
	}
}

func TestArrowV1Mapper_Map_MissingMetadata(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{
		"schema": "arrow@v1",
	}

	_, err := mapper.Map(yamlData)
	if err == nil {
		t.Error("Map() should return error for missing metadata")
	}
}

func TestArrowV1Mapper_Map_WrongMetadataType(t *testing.T) {
	mapper := NewMapper()

	yamlData := map[string]interface{}{
		"schema":   "arrow@v1",
		"metadata": "not-a-map",
	}

	_, err := mapper.Map(yamlData)
	if err == nil {
		t.Error("Map() should return error for wrong metadata type")
	}
}

func TestMapRequirements_WithSystem(t *testing.T) {
	req := map[string]interface{}{
		"cpu_cores":    float64(4),
		"ram_gb":       float64(8),
		"disk_gb":      float64(20),
		"network_mbps": float64(50),
		"system":       []interface{}{"windows/amd64", "linux/amd64"},
	}

	result := mapRequirements(req)

	if result.CpuCores != 4 {
		t.Errorf("CpuCores = %d, want 4", result.CpuCores)
	}
	if result.OS != "windows/amd64" {
		t.Errorf("OS = %s, want windows/amd64", result.OS)
	}
}

func TestMapRequirements_EmptySystem(t *testing.T) {
	req := map[string]interface{}{
		"cpu_cores": float64(2),
		"system":    []interface{}{},
	}

	result := mapRequirements(req)

	if result.OS != "" {
		t.Errorf("OS = %s, want empty", result.OS)
	}
}

func TestMapRequirements_InvalidSystemType(t *testing.T) {
	req := map[string]interface{}{
		"cpu_cores": float64(2),
		"system":    []interface{}{123, 456},
	}

	result := mapRequirements(req)

	if result.OS != "" {
		t.Errorf("OS = %s, want empty", result.OS)
	}
}

func TestMapNetbridge_AllProtocols(t *testing.T) {
	netbridgeData := []interface{}{
		map[string]interface{}{
			"name":     "TCP_PORT",
			"protocol": "tcp",
		},
		map[string]interface{}{
			"name":     "UDP_PORT",
			"protocol": "udp",
		},
		map[string]interface{}{
			"name":     "BOTH_PORT",
			"protocol": "tcp/udp",
		},
		map[string]interface{}{
			"name":     "UNKNOWN_PORT",
			"protocol": "unknown",
		},
	}

	result := mapNetbridge(netbridgeData)

	if len(result) != 4 {
		t.Errorf("got %d rules, want 4", len(result))
	}
}

func TestMapNetbridge_InvalidData(t *testing.T) {
	netbridgeData := []interface{}{
		"not-a-map",
		123,
		nil,
	}

	result := mapNetbridge(netbridgeData)

	if len(result) != 0 {
		t.Errorf("got %d rules, want 0", len(result))
	}
}

func TestMapVariables_CompleteData(t *testing.T) {
	variables := []interface{}{
		map[string]interface{}{
			"name":      "VAR1",
			"default":   "default1",
			"sensitive": true,
			"values":    []interface{}{"val1", "val2"},
			"min":       float64(5),
			"max":       float64(15),
		},
	}

	result := mapVariables(variables)

	if len(result) != 1 {
		t.Fatalf("got %d variables, want 1", len(result))
	}
	if result[0].Name != "VAR1" {
		t.Errorf("Name = %s, want VAR1", result[0].Name)
	}
	if !result[0].Sensitive {
		t.Error("Sensitive should be true")
	}
	if result[0].Min != 5 {
		t.Errorf("Min = %d, want 5", result[0].Min)
	}
	if result[0].Max != 15 {
		t.Errorf("Max = %d, want 15", result[0].Max)
	}
}

func TestMapVariables_InvalidData(t *testing.T) {
	variables := []interface{}{
		"not-a-map",
		123,
		nil,
	}

	result := mapVariables(variables)

	if len(result) != 0 {
		t.Errorf("got %d variables, want 0", len(result))
	}
}

func TestMapWizards_InvalidWizard(t *testing.T) {
	wizards := []interface{}{
		"not-a-map",
		map[string]interface{}{
			"platforms": []interface{}{"linux"},
			"methods":   "not-an-array",
		},
	}

	result := mapWizards(wizards)

	if len(result) != 0 {
		t.Errorf("got %d methods, want 0", len(result))
	}
}

func TestMapWizards_InvalidMethod(t *testing.T) {
	wizards := []interface{}{
		map[string]interface{}{
			"platforms": []interface{}{"linux"},
			"methods": []interface{}{
				"not-a-map",
				123,
			},
		},
	}

	result := mapWizards(wizards)

	if len(result) != 0 {
		t.Errorf("got %d methods, want 0", len(result))
	}
}

func TestMapActions_AllTypes(t *testing.T) {
	actionsList := []interface{}{
		map[string]interface{}{
			"name":            "Run Action",
			"run":             "echo test",
			"exit_on_failure": true,
			"timeout":         "30s",
			"to":              "/path",
		},
		map[string]interface{}{
			"name":     "Download Action",
			"download": "https://example.com/file.tar.gz",
		},
		map[string]interface{}{
			"name": "Copy Action",
			"copy": "source.txt",
			"to":   "dest.txt",
		},
		map[string]interface{}{
			"name":       "Uncompress Action",
			"uncompress": "file.tar.gz",
			"to":         "/extract/path",
		},
	}

	result := mapActions(actionsList)

	if len(result) != 4 {
		t.Fatalf("got %d actions, want 4", len(result))
	}

	if result[0].Type != "run" {
		t.Errorf("Action 0 Type = %s, want run", result[0].Type)
	}
	if result[0].Value != "echo test" {
		t.Errorf("Action 0 Value = %s, want 'echo test'", result[0].Value)
	}
	if !result[0].ExitOnFailure {
		t.Error("Action 0 ExitOnFailure should be true")
	}

	if result[1].Type != "download" {
		t.Errorf("Action 1 Type = %s, want download", result[1].Type)
	}

	if result[2].Type != "copy" {
		t.Errorf("Action 2 Type = %s, want copy", result[2].Type)
	}

	if result[3].Type != "uncompress" {
		t.Errorf("Action 3 Type = %s, want uncompress", result[3].Type)
	}
}

func TestMapActions_InvalidData(t *testing.T) {
	actionsList := []interface{}{
		"not-a-map",
		123,
		nil,
		map[string]interface{}{
			"name": "No Action Type",
		},
	}

	result := mapActions(actionsList)

	if len(result) != 1 {
		t.Errorf("got %d actions, want 1", len(result))
	}
}

func TestMapActions_EmptyList(t *testing.T) {
	actionsList := []interface{}{}

	result := mapActions(actionsList)

	if len(result) != 0 {
		t.Errorf("got %d actions, want 0", len(result))
	}
}

func TestNewMapper(t *testing.T) {
	mapper := NewMapper()
	if mapper == nil {
		t.Fatal("NewMapper() returned nil")
	}
}
