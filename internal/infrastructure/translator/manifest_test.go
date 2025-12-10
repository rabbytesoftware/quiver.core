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
			wantErr:     false,
		},
		{
			name:        "valid quiver manifest",
			input:       "quiver@v1",
			wantType:    "quiver",
			wantVersion: "v1",
			wantKey:     "quiver@v1",
			wantErr:     false,
		},
		{
			name:        "manifest with quotes",
			input:       "\"arrow@v1\"",
			wantType:    "arrow",
			wantVersion: "v1",
			wantKey:     "arrow@v1",
			wantErr:     false,
		},
		{
			name:        "manifest with single quotes",
			input:       "'arrow@v1'",
			wantType:    "arrow",
			wantVersion: "v1",
			wantKey:     "arrow@v1",
			wantErr:     false,
		},
		{
			name:        "manifest with whitespace",
			input:       "  arrow@v1  ",
			wantType:    "arrow",
			wantVersion: "v1",
			wantKey:     "arrow@v1",
			wantErr:     false,
		},
		{
			name:    "missing @ separator",
			input:   "arrowv1",
			wantErr: true,
		},
		{
			name:    "multiple @ separators",
			input:   "arrow@v1@extra",
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
			name:    "only @",
			input:   "@",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "whitespace before @",
			input:   "arrow @v1",
			wantErr: false,
			wantType: "arrow",
			wantVersion: "v1",
			wantKey: "arrow@v1",
		},
		{
			name:    "whitespace after @",
			input:   "arrow@ v1",
			wantErr: false,
			wantType: "arrow",
			wantVersion: "v1",
			wantKey: "arrow@v1",
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
			wantErr:     false,
		},
		{
			name:        "valid quiver schema",
			yamlData:    []byte("schema: quiver@v1\nmetadata:\n  name: test"),
			wantType:    "quiver",
			wantVersion: "v1",
			wantErr:     false,
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
			name:     "empty YAML",
			yamlData: []byte(""),
			wantErr:  true,
		},
		{
			name:     "invalid manifest format",
			yamlData: []byte("schema: invalid-format"),
			wantErr:  true,
		},
		{
			name:        "schema with quotes",
			yamlData:    []byte("schema: \"arrow@v1\""),
			wantType:    "arrow",
			wantVersion: "v1",
			wantErr:     false,
		},
		{
			name:     "malformed YAML structure",
			yamlData: []byte("schema:\n  - invalid\n  - structure"),
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
			wantErr:  false,
		},
		{
			name:     "valid manifest field",
			yamlData: []byte("manifest: quiver@v1"),
			want:     "quiver@v1",
			wantErr:  false,
		},
		{
			name:     "schema field takes precedence",
			yamlData: []byte("schema: arrow@v1\nmanifest: quiver@v1"),
			want:     "arrow@v1",
			wantErr:  false,
		},
		{
			name:     "empty schema field",
			yamlData: []byte("schema: \"\""),
			want:     "",
			wantErr:  false,
		},
		{
			name:     "missing schema field",
			yamlData: []byte("other: value"),
			want:     "",
			wantErr:  false,
		},
		{
			name:     "invalid YAML",
			yamlData: []byte("invalid: [[["),
			wantErr:  true,
		},
		{
			name:     "empty YAML",
			yamlData: []byte(""),
			want:     "",
			wantErr:  false,
		},
		{
			name:     "schema with complex value",
			yamlData: []byte("schema: quiver@v2"),
			want:     "quiver@v2",
			wantErr:  false,
		},
		{
			name:     "manifest field only",
			yamlData: []byte("manifest: quiver@v1\nmetadata:\n  name: test"),
			want:     "quiver@v1",
			wantErr:  false,
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
