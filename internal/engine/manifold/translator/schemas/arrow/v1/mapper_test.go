package v1

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
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
			"cpu_cores": float64(2),
			"ram_gb":    float64(4),
			"disk_gb":   float64(10),
			"system":    []interface{}{"linux/amd64"},
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

	if arrow.Manifest.Name != "test-arrow" {
		t.Errorf("got name %s, want test-arrow", arrow.Manifest.Name)
	}
	if arrow.Manifest.Version != "1.0.0" {
		t.Errorf("got version %s, want 1.0.0", arrow.Manifest.Version)
	}
	if arrow.Manifest.URL != "github.com/test/test" {
		t.Errorf("got URL %s, want github.com/test/test", arrow.Manifest.URL)
	}
	if len(arrow.Manifest.Variables) > 0 {
		if len(arrow.Manifest.Variables[0].Values) != 2 {
			t.Errorf("got %d values, want 2", len(arrow.Manifest.Variables[0].Values))
		}
	}
	if len(arrow.Manifest.Lifecycle.Install) == 0 {
		t.Error("expected at least one install lifecycle step")
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
			"cpu_cores": float64(1),
			"ram_gb":    float64(1),
			"disk_gb":   float64(1),
			"system":    []interface{}{"linux/amd64"},
		},
	}

	arrow, err := mapper.Map(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if arrow.Manifest.Name != "minimal-arrow" {
		t.Errorf("got name %s, want minimal-arrow", arrow.Manifest.Name)
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
		"cpu_cores": float64(4),
		"ram_gb":    float64(8),
		"disk_gb":   float64(20),
		"system":    []interface{}{"windows/amd64", "linux/amd64"},
	}

	result := mapRequirements(req)

	if result.CpuCores != 4 {
		t.Errorf("CpuCores = %d, want 4", result.CpuCores)
	}
	if len(result.OS) == 0 || result.OS[0] != "windows/amd64" {
		t.Errorf("OS[0] = %v, want windows/amd64", result.OS)
	}
}

func TestMapRequirements_EmptySystem(t *testing.T) {
	req := map[string]interface{}{
		"cpu_cores": float64(2),
		"system":    []interface{}{},
	}

	result := mapRequirements(req)

	if len(result.OS) != 0 {
		t.Errorf("OS = %v, want empty slice", result.OS)
	}
}

func TestMapRequirements_InvalidSystemType(t *testing.T) {
	req := map[string]interface{}{
		"cpu_cores": float64(2),
		"system":    []interface{}{123, 456},
	}

	result := mapRequirements(req)

	if len(result.OS) != 0 {
		t.Errorf("OS = %v, want empty slice", result.OS)
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

	lifecycle, methods := mapWizards(wizards)

	totalSteps := len(lifecycle.Install) + len(lifecycle.Execute) + len(lifecycle.Stop) + len(lifecycle.Uninstall)
	if totalSteps != 0 || len(methods) != 0 {
		t.Errorf("got %d lifecycle steps and %d methods, want 0 each", totalSteps, len(methods))
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

	lifecycle, methods := mapWizards(wizards)

	totalSteps := len(lifecycle.Install) + len(lifecycle.Execute) + len(lifecycle.Stop) + len(lifecycle.Uninstall)
	if totalSteps != 0 || len(methods) != 0 {
		t.Errorf("got %d lifecycle steps and %d methods, want 0 each", totalSteps, len(methods))
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
		t.Fatalf("got %d steps, want 4", len(result))
	}

	if result[0].Type() != step.StepTypeRun {
		t.Errorf("Step 0 Type = %s, want run", result[0].Type())
	}
	if result[0].(step.RunStep).Command != "echo test" {
		t.Errorf("Step 0 Command = %s, want 'echo test'", result[0].(step.RunStep).Command)
	}
	if !result[0].ExitOnFailure() {
		t.Error("Step 0 ExitOnFailure should be true")
	}

	if result[1].Type() != step.StepTypeFetch {
		t.Errorf("Step 1 Type = %s, want fetch", result[1].Type())
	}

	// copy and uncompress are mapped to RunStep
	if result[2].Type() != step.StepTypeRun {
		t.Errorf("Step 2 Type = %s, want run (copy mapped to run)", result[2].Type())
	}

	if result[3].Type() != step.StepTypeRun {
		t.Errorf("Step 3 Type = %s, want run (uncompress mapped to run)", result[3].Type())
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
		t.Errorf("got %d steps, want 1", len(result))
	}
}

func TestMapActions_EmptyList(t *testing.T) {
	actionsList := []interface{}{}

	result := mapActions(actionsList)

	if len(result) != 0 {
		t.Errorf("got %d steps, want 0", len(result))
	}
}

func TestNewMapper(t *testing.T) {
	mapper := NewMapper()
	if mapper == nil {
		t.Fatal("NewMapper() returned nil")
	}
}
