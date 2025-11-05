package parser

import (
	"testing"
)

func TestParser_Parse(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name        string
		manifestStr string
		wantType    string
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "valid arrow v1",
			manifestStr: "arrow@v1",
			wantType:    "arrow",
			wantVersion: "v1",
			wantErr:     false,
		},
		{
			name:        "valid quiver v1",
			manifestStr: "quiver@v1",
			wantType:    "quiver",
			wantVersion: "v1",
			wantErr:     false,
		},
		{
			name:        "invalid format no separator",
			manifestStr: "arrowv1",
			wantErr:     true,
		},
		{
			name:        "invalid format multiple separators",
			manifestStr: "arrow@v1@extra",
			wantErr:     true,
		},
		{
			name:        "empty schema type",
			manifestStr: "@v1",
			wantErr:     true,
		},
		{
			name:        "empty version",
			manifestStr: "arrow@",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := parser.Parse(tt.manifestStr)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if info.SchemaType != tt.wantType {
				t.Errorf("got type %s, want %s", info.SchemaType, tt.wantType)
			}
			if info.Version != tt.wantVersion {
				t.Errorf("got version %s, want %s", info.Version, tt.wantVersion)
			}
			if info.ManifestKey != tt.manifestStr {
				t.Errorf("got key %s, want %s", info.ManifestKey, tt.manifestStr)
			}
		})
	}
}

func TestParser_ParseFromYAML(t *testing.T) {
	parser := NewParser()

	yamlData := []byte(`manifest: "arrow@v1"
metadata:
  name: test`)

	info, err := parser.ParseFromYAML(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.SchemaType != "arrow" {
		t.Errorf("got type %s, want arrow", info.SchemaType)
	}
	if info.Version != "v1" {
		t.Errorf("got version %s, want v1", info.Version)
	}
}
