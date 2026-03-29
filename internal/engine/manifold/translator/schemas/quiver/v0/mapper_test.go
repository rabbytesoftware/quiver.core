package v0_test

import (
	"testing"

	v0 "github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/schemas/quiver/v0"
)

var minimalQuiverYAML = []byte(`
schema: "quiver@v0"
name: "Gaming Quiver"
description: "Curated game servers"
`)

var fullQuiverYAML = []byte(`
schema: "quiver@v0"
name: "Gaming Quiver"
description: "Game servers curated by char2cs"
url: "https://gaming.quiver.ar"
maintainers:
  - char2cs
tags:
  - gaming
  - servers
media:
  icon: "https://example.com/icon.png"
  banner: "https://example.com/banner.png"
arrows:
  - cs2
  - github.com/valve/steamcmd
`)

func TestQuiverV0Mapper_Map_Minimal(t *testing.T) {
	m := v0.NewMapper()
	raw, err := m.Map(minimalQuiverYAML)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if raw.Name != "Gaming Quiver" {
		t.Errorf("Name = %q, want %q", raw.Name, "Gaming Quiver")
	}
	if raw.Description != "Curated game servers" {
		t.Errorf("Description = %q, want %q", raw.Description, "Curated game servers")
	}
}

func TestQuiverV0Mapper_Map_Full(t *testing.T) {
	m := v0.NewMapper()
	raw, err := m.Map(fullQuiverYAML)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if raw.URL != "https://gaming.quiver.ar" {
		t.Errorf("URL = %q, want https://gaming.quiver.ar", raw.URL)
	}
	if len(raw.Maintainers) != 1 {
		t.Errorf("Maintainers count = %d, want 1", len(raw.Maintainers))
	}
	if len(raw.Tags) != 2 {
		t.Errorf("Tags count = %d, want 2", len(raw.Tags))
	}
	if raw.Media.Icon != "https://example.com/icon.png" {
		t.Errorf("Media.Icon = %q, want https://example.com/icon.png", raw.Media.Icon)
	}
	if len(raw.Arrows) != 2 {
		t.Errorf("Arrows count = %d, want 2", len(raw.Arrows))
	}
}

func TestQuiverV0Mapper_Map_InvalidYAML(t *testing.T) {
	m := v0.NewMapper()
	_, err := m.Map([]byte("not: valid: yaml: :"))
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestQuiverV0Mapper_GetSchema(t *testing.T) {
	m := v0.NewMapper()
	schema, err := m.GetSchema()
	if err != nil {
		t.Fatalf("GetSchema() error = %v", err)
	}
	if len(schema) == 0 {
		t.Error("GetSchema() returned empty schema")
	}
}
