package schemas

import (
	"testing"
)

func TestRegistry_IsSupported(t *testing.T) {
	registry := NewRegistry()

	tests := []struct {
		name        string
		manifestKey string
		want        bool
	}{
		{"arrow v1 supported", "arrow@v1", true},
		{"quiver v1 supported", "quiver@v1", true},
		{"arrow v2 not supported", "arrow@v2", false},
		{"invalid key", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := registry.IsSupported(tt.manifestKey)
			if got != tt.want {
				t.Errorf("IsSupported(%s) = %v, want %v", tt.manifestKey, got, tt.want)
			}
		})
	}
}

func TestRegistry_GetArrowMapper(t *testing.T) {
	registry := NewRegistry()

	mapper, err := registry.GetArrowMapper("arrow@v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mapper == nil {
		t.Error("expected mapper, got nil")
	}

	_, err = registry.GetArrowMapper("arrow@v999")
	if err == nil {
		t.Error("expected error for unsupported version")
	}
}

func TestRegistry_GetSchema(t *testing.T) {
	registry := NewRegistry()

	schema, err := registry.GetSchema("arrow@v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(schema) == 0 {
		t.Error("expected schema content")
	}
}
