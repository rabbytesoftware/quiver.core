package translator

import (
	"testing"
)

func TestParseManifestString(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantType    string
		wantVersion string
		wantKey     string
		wantErr     bool
	}{
		{
			name:        "valid arrow manifest",
			input:       "arrow@v1",
			wantType:    "arrow",
			wantVersion: "v1",
			wantKey:     "arrow@v1",
		},
		{
			name:        "manifest with double quotes",
			input:       "\"arrow@v1\"",
			wantType:    "arrow",
			wantVersion: "v1",
			wantKey:     "arrow@v1",
		},
		{
			name:        "manifest with whitespace",
			input:       "  arrow@v1  ",
			wantType:    "arrow",
			wantVersion: "v1",
			wantKey:     "arrow@v1",
		},
		{
			name:    "missing @ separator",
			input:   "arrowv1",
			wantErr: true,
		},
		{
			name:    "empty schema type",
			input:   "@v1",
			wantErr: true,
		},
		{
			name:    "empty version",
			input:   "arrow@",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseManifestString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseManifestString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.SchemaType != tt.wantType {
					t.Errorf("SchemaType = %v, want %v", got.SchemaType, tt.wantType)
				}
				if got.Version != tt.wantVersion {
					t.Errorf("Version = %v, want %v", got.Version, tt.wantVersion)
				}
				if got.ManifestKey != tt.wantKey {
					t.Errorf("ManifestKey = %v, want %v", got.ManifestKey, tt.wantKey)
				}
			}
		})
	}
}

func TestExtractManifestFromYAML(t *testing.T) {
	tests := []struct {
		name        string
		yamlData    []byte
		wantType    string
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "valid arrow schema",
			yamlData:    []byte("schema: arrow@v1\nmetadata:\n  name: test"),
			wantType:    "arrow",
			wantVersion: "v1",
		},
		{
			name:     "missing schema field",
			yamlData: []byte("metadata:\n  name: test"),
			wantErr:  true,
		},
		{
			name:     "empty schema field",
			yamlData: []byte("schema: \"\"\nmetadata:\n  name: test"),
			wantErr:  true,
		},
		{
			name:     "invalid YAML",
			yamlData: []byte("invalid: yaml: content: [[["),
			wantErr:  true,
		},
		{
			name:     "invalid manifest format",
			yamlData: []byte("schema: invalid-format"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractManifestFromYAML(tt.yamlData)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractManifestFromYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.SchemaType != tt.wantType {
					t.Errorf("SchemaType = %v, want %v", got.SchemaType, tt.wantType)
				}
				if got.Version != tt.wantVersion {
					t.Errorf("Version = %v, want %v", got.Version, tt.wantVersion)
				}
			}
		})
	}
}

func TestExtractSchemaField(t *testing.T) {
	tests := []struct {
		name     string
		yamlData []byte
		want     string
		wantErr  bool
	}{
		{
			name:     "valid schema field",
			yamlData: []byte("schema: arrow@v1"),
			want:     "arrow@v1",
		},
		{
			name:     "schema field takes precedence over manifest",
			yamlData: []byte("schema: arrow@v1\nmanifest: quiver@v1"),
			want:     "arrow@v1",
		},
		{
			name:     "missing schema field returns empty",
			yamlData: []byte("other: value"),
			want:     "",
		},
		{
			name:    "invalid YAML",
			yamlData: []byte("invalid: [[["),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractSchemaField(tt.yamlData)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractSchemaField() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractSchemaField() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ─── YAML validation tests ────────────────────────────────────────────────────

func TestValidateYAML(t *testing.T) {
	tests := []struct {
		name       string
		schemaJSON []byte
		yamlData   []byte
		wantErr    bool
	}{
		{
			name: "valid data passes schema",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"name": {"type": "string"}
				},
				"required": ["name"]
			}`),
			yamlData: []byte("name: test"),
			wantErr:  false,
		},
		{
			name: "missing required field",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"name": {"type": "string"}
				},
				"required": ["name"]
			}`),
			yamlData: []byte("other: value"),
			wantErr:  true,
		},
		{
			name:       "invalid YAML data",
			schemaJSON: []byte(`{"type": "object"}`),
			yamlData:   []byte("invalid: yaml: [[["),
			wantErr:    true,
		},
		{
			name:       "invalid schema JSON",
			schemaJSON: []byte(`{invalid json`),
			yamlData:   []byte("name: test"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateYAML(tt.schemaJSON, tt.yamlData)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateYAML() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
