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
		{"arrow v0 supported", "arrow@v0", true},
		{"quiver v0 supported", "quiver@v0", true},
		{"arrow v1 not supported", "arrow@v1", false},
		{"quiver v1 not supported", "quiver@v1", false},
		{"invalid key", "invalid", false},
		{"empty key", "", false},
		{"arrow without version", "arrow@", false},
		{"quiver without version", "quiver@", false},
		{"unknown schema type", "unknown@v0", false},
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

	tests := []struct {
		name        string
		manifestKey string
		wantErr     bool
	}{
		{"valid arrow v0", "arrow@v0", false},
		{"unsupported arrow version", "arrow@v999", true},
		{"invalid key", "invalid", true},
		{"empty key", "", true},
		{"quiver key", "quiver@v0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper, err := registry.GetArrowMapper(tt.manifestKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetArrowMapper(%s) error = %v, wantErr %v", tt.manifestKey, err, tt.wantErr)
			}
			if !tt.wantErr && mapper == nil {
				t.Error("expected mapper, got nil")
			}
		})
	}
}

func TestRegistry_GetQuiverMapper(t *testing.T) {
	registry := NewRegistry()

	tests := []struct {
		name        string
		manifestKey string
		wantErr     bool
	}{
		{"valid quiver v0", "quiver@v0", false},
		{"unsupported quiver version", "quiver@v999", true},
		{"invalid key", "invalid", true},
		{"empty key", "", true},
		{"arrow key", "arrow@v0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper, err := registry.GetQuiverMapper(tt.manifestKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetQuiverMapper(%s) error = %v, wantErr %v", tt.manifestKey, err, tt.wantErr)
			}
			if !tt.wantErr && mapper == nil {
				t.Error("expected mapper, got nil")
			}
		})
	}
}

func TestRegistry_GetSchema(t *testing.T) {
	registry := NewRegistry()

	tests := []struct {
		name        string
		manifestKey string
		wantErr     bool
	}{
		{"arrow v0 schema", "arrow@v0", false},
		{"quiver v0 schema", "quiver@v0", false},
		{"unsupported arrow version", "arrow@v999", true},
		{"unsupported quiver version", "quiver@v999", true},
		{"invalid key", "invalid@v0", true},
		{"empty key", "", true},
		{"unknown schema type", "unknown@v0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := registry.GetSchema(tt.manifestKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSchema(%s) error = %v, wantErr %v", tt.manifestKey, err, tt.wantErr)
			}
			if !tt.wantErr && len(schema) == 0 {
				t.Error("expected schema content")
			}
		})
	}
}

func TestRegistry_NewRegistry(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Fatal("NewRegistry() returned nil")
	}
	if registry.arrowMappers == nil {
		t.Error("arrowMappers not initialized")
	}
	if registry.quiverMappers == nil {
		t.Error("quiverMappers not initialized")
	}
	if len(registry.arrowMappers) == 0 {
		t.Error("no arrow mappers registered")
	}
	if len(registry.quiverMappers) == 0 {
		t.Error("no quiver mappers registered")
	}
}
