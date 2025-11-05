package validator

import (
	"testing"
)

func TestValidator_Validate(t *testing.T) {
	validator := NewValidator()

	schema := []byte(`{
		"type": "object",
		"required": ["name"],
		"properties": {
			"name": {"type": "string"}
		}
	}`)

	tests := []struct {
		name      string
		yamlData  []byte
		wantValid bool
	}{
		{
			name:      "valid yaml",
			yamlData:  []byte("name: test"),
			wantValid: true,
		},
		{
			name:      "missing required field",
			yamlData:  []byte("other: value"),
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.Validate(schema, tt.yamlData)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Valid != tt.wantValid {
				t.Errorf("got valid=%v, want %v", result.Valid, tt.wantValid)
			}
		})
	}
}
