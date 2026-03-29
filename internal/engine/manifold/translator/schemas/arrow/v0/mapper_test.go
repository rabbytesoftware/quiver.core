package v0_test

import (
	"testing"

	v0 "github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/schemas/arrow/v0"
)

var minimalArrowYAML = []byte(`
schema: "arrow@v0"
metadata:
  name: test-arrow
  version: 1.0.0
`)

var fullArrowYAML = []byte(`
schema: "arrow@v0"
metadata:
  name: cs2-server
  description: CS2 Dedicated Server
  version: 25.11.0
  license: MIT
  quiver: github.com/rabbytesoftware/quiver.experiments
  media:
    icon: https://example.com/icon.png
    banner: https://example.com/banner.png
  credits:
    - name: char2cs
      email: me@char2cs.net
      url: https://char2cs.net
  maintainers:
    - char2cs
  tags:
    - gaming

requirements:
  cpu_cores: 2
  ram_gb: 8
  disk_gb: 60
  os:
    - linux/amd64
    - linux/arm64

dependencies:
  - github.com/valvesoftware/quiver/steamcmd

variables:
  - name: CS2_PORT
    default: "27015"
    sensitive: false

netbridge:
  - name: GAME_PORT
    protocol: tcp/udp
    default: 27015
    required: true

lifecycle:
  install:
    - type: run
      command: "${steamcmd} +app_update 730"
      title: "Installing CS2"
      timeout: 30m
  execute:
    - type: run
      command: "./srcds_run -game csgo"
      title: "Starting CS2"
  stop:
    - type: signal
      signal: SIGTERM
      timeout: 30s
  uninstall:
    - type: run
      command: "./cleanup.sh"

methods:
  update:
    available_in: [ready]
    steps:
      - type: run
        command: "${steamcmd} +app_update 730"
        title: "Updating CS2"
`)

func TestArrowV0Mapper_Map_Minimal(t *testing.T) {
	m := v0.NewMapper()
	raw, err := m.Map(minimalArrowYAML)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if raw.Metadata.Name != "test-arrow" {
		t.Errorf("Name = %q, want %q", raw.Metadata.Name, "test-arrow")
	}
	if raw.Metadata.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", raw.Metadata.Version, "1.0.0")
	}
}

func TestArrowV0Mapper_Map_Full(t *testing.T) {
	m := v0.NewMapper()
	raw, err := m.Map(fullArrowYAML)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if raw.Metadata.Name != "cs2-server" {
		t.Errorf("Name = %q, want cs2-server", raw.Metadata.Name)
	}
	if raw.Requirements.CpuCores != 2 {
		t.Errorf("CpuCores = %d, want 2", raw.Requirements.CpuCores)
	}
	if raw.Requirements.RamGB != 8 {
		t.Errorf("RamGB = %d, want 8", raw.Requirements.RamGB)
	}
	if len(raw.Requirements.OS) != 2 {
		t.Errorf("OS count = %d, want 2", len(raw.Requirements.OS))
	}
	if len(raw.Dependencies) != 1 {
		t.Errorf("Dependencies count = %d, want 1", len(raw.Dependencies))
	}
	if len(raw.Variables) != 1 {
		t.Errorf("Variables count = %d, want 1", len(raw.Variables))
	}
	if len(raw.Netbridge) != 1 {
		t.Errorf("Netbridge count = %d, want 1", len(raw.Netbridge))
	}
	if len(raw.Lifecycle.Install) != 1 {
		t.Errorf("Lifecycle.Install count = %d, want 1", len(raw.Lifecycle.Install))
	}
	if len(raw.Lifecycle.Execute) != 1 {
		t.Errorf("Lifecycle.Execute count = %d, want 1", len(raw.Lifecycle.Execute))
	}
	if len(raw.Lifecycle.Stop) != 1 {
		t.Errorf("Lifecycle.Stop count = %d, want 1", len(raw.Lifecycle.Stop))
	}
	if len(raw.Lifecycle.Uninstall) != 1 {
		t.Errorf("Lifecycle.Uninstall count = %d, want 1", len(raw.Lifecycle.Uninstall))
	}
	if _, ok := raw.Methods["update"]; !ok {
		t.Error("expected 'update' method")
	}
}

func TestArrowV0Mapper_Map_InvalidYAML(t *testing.T) {
	m := v0.NewMapper()
	_, err := m.Map([]byte("not: valid: yaml: :"))
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestArrowV0Mapper_GetSchema(t *testing.T) {
	m := v0.NewMapper()
	schema, err := m.GetSchema()
	if err != nil {
		t.Fatalf("GetSchema() error = %v", err)
	}
	if len(schema) == 0 {
		t.Error("GetSchema() returned empty schema")
	}
}
