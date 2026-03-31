package v0_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	v0 "github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/quiver/v0"
)

func TestModule_Version(t *testing.T) {
	if v0.Default.Version() != "v0" {
		t.Errorf("Version() = %q, want v0", v0.Default.Version())
	}
}

func TestModule_GetSchema(t *testing.T) {
	schema, err := v0.Default.GetSchema()
	if err != nil {
		t.Fatalf("GetSchema() error = %v", err)
	}
	if len(schema) == 0 {
		t.Error("GetSchema() returned empty schema")
	}
}

var minimalQuiverYAML = []byte(`
schema: "quiver@v0"
name: test-quiver
description: A test quiver
`)

func TestModule_Map_Minimal(t *testing.T) {
	result, err := v0.Default.Map(minimalQuiverYAML)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if result.Name != "test-quiver" {
		t.Errorf("Name = %q, want test-quiver", result.Name)
	}
	if result.Description != "A test quiver" {
		t.Errorf("Description = %q, want A test quiver", result.Description)
	}
}

var fullQuiverYAML = []byte(`
schema: "quiver@v0"
name: gaming-quiver
description: A collection of gaming tools
url: https://gaming.example.com
maintainers:
  - gaming-team
  - community
tags:
  - gaming
  - tools
media:
  icon: https://example.com/icon.png
  banner: https://example.com/banner.png
arrows:
  - github.com/game/arrow1
  - github.com/game/arrow2
`)

func TestModule_Map_Full(t *testing.T) {
	result, err := v0.Default.Map(fullQuiverYAML)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if result.Name != "gaming-quiver" {
		t.Errorf("Name = %q, want gaming-quiver", result.Name)
	}
	if result.Description != "A collection of gaming tools" {
		t.Errorf("Description mismatch")
	}
	if result.URL != "https://gaming.example.com" {
		t.Errorf("URL = %q", result.URL)
	}

	if len(result.Maintainers) != 2 {
		t.Errorf("Maintainers count = %d, want 2", len(result.Maintainers))
	}
	if result.Maintainers[0] != "gaming-team" {
		t.Errorf("Maintainer 0 = %q", result.Maintainers[0])
	}

	if len(result.Tags) != 2 {
		t.Errorf("Tags count = %d, want 2", len(result.Tags))
	}

	if result.Media.Icon != "https://example.com/icon.png" {
		t.Errorf("Icon = %q", result.Media.Icon)
	}
	if result.Media.Banner != "https://example.com/banner.png" {
		t.Errorf("Banner = %q", result.Media.Banner)
	}

	if len(result.Arrows) != 2 {
		t.Errorf("Arrows count = %d, want 2", len(result.Arrows))
	}
	if result.Arrows[0] != domain.Namespace("github.com/game/arrow1") {
		t.Errorf("Arrow 0 = %q", result.Arrows[0])
	}
	if result.Arrows[1] != domain.Namespace("github.com/game/arrow2") {
		t.Errorf("Arrow 1 = %q", result.Arrows[1])
	}
}

func TestModule_Map_InvalidYAML(t *testing.T) {
	_, err := v0.Default.Map([]byte("not: valid: yaml: :"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestModule_Map_EmptyOptionalFields(t *testing.T) {
	yamlData := []byte(`
schema: "quiver@v0"
name: minimal-quiver
description: Minimal description
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if result.URL != "" {
		t.Errorf("URL should be empty, got %q", result.URL)
	}
	if len(result.Maintainers) != 0 {
		t.Errorf("Maintainers should be empty")
	}
	if len(result.Tags) != 0 {
		t.Errorf("Tags should be empty")
	}
	if len(result.Arrows) != 0 {
		t.Errorf("Arrows should be empty")
	}
}

func TestModule_Map_ArrowsWithSpecialChars(t *testing.T) {
	yamlData := []byte(`
schema: "quiver@v0"
name: special-arrows
description: Test
arrows:
  - github.com/org/arrow-with-dashes
  - github.com/org/arrow.with.dots
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(result.Arrows) != 2 {
		t.Errorf("Arrows count = %d, want 2", len(result.Arrows))
	}
}

func TestModule_Map_MultipleArrows(t *testing.T) {
	yamlData := []byte(`
schema: "quiver@v0"
name: multi-arrow
description: Multiple arrows
arrows:
  - github.com/org/arrow1
  - github.com/org/arrow2
  - github.com/org/arrow3
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(result.Arrows) != 3 {
		t.Errorf("Arrows count = %d, want 3", len(result.Arrows))
	}
}

func TestModule_Map_WithoutMedia(t *testing.T) {
	yamlData := []byte(`
schema: "quiver@v0"
name: no-media
description: Quiver without media
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if result.Media.Icon != "" {
		t.Errorf("Icon should be empty")
	}
	if result.Media.Banner != "" {
		t.Errorf("Banner should be empty")
	}
}

func TestModule_Map_WithMedia(t *testing.T) {
	yamlData := []byte(`
schema: "quiver@v0"
name: with-media
description: Quiver with media
media:
  icon: https://example.com/icon.png
  banner: https://example.com/banner.png
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if result.Media.Icon != "https://example.com/icon.png" {
		t.Errorf("Icon = %q, want https://example.com/icon.png", result.Media.Icon)
	}
	if result.Media.Banner != "https://example.com/banner.png" {
		t.Errorf("Banner = %q, want https://example.com/banner.png", result.Media.Banner)
	}
}

func TestModule_Map_WithMaintainers(t *testing.T) {
	yamlData := []byte(`
schema: "quiver@v0"
name: with-maintainers
description: Quiver with maintainers
maintainers:
  - alice
  - bob
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(result.Maintainers) != 2 {
		t.Errorf("Maintainers count = %d, want 2", len(result.Maintainers))
	}
	if result.Maintainers[0] != "alice" {
		t.Errorf("First maintainer = %q, want alice", result.Maintainers[0])
	}
}

func TestModule_Map_WithTags(t *testing.T) {
	yamlData := []byte(`
schema: "quiver@v0"
name: with-tags
description: Quiver with tags
tags:
  - backend
  - database
  - cache
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(result.Tags) != 3 {
		t.Errorf("Tags count = %d, want 3", len(result.Tags))
	}
}

func TestModule_Map_AllFields(t *testing.T) {
	yamlData := []byte(`
schema: "quiver@v0"
name: full-quiver
description: A complete quiver
url: https://example.com
maintainers:
  - alice
tags:
  - test
media:
  icon: https://example.com/icon.png
  banner: https://example.com/banner.png
arrows:
  - github.com/org/arrow1
  - github.com/org/arrow2
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if result.Name != "full-quiver" {
		t.Errorf("Name = %q, want full-quiver", result.Name)
	}
	if result.Description != "A complete quiver" {
		t.Errorf("Description = %q", result.Description)
	}
	if result.URL != "https://example.com" {
		t.Errorf("URL = %q", result.URL)
	}
	if len(result.Maintainers) != 1 {
		t.Errorf("Maintainers count = %d", len(result.Maintainers))
	}
	if len(result.Tags) != 1 {
		t.Errorf("Tags count = %d", len(result.Tags))
	}
	if len(result.Arrows) != 2 {
		t.Errorf("Arrows count = %d", len(result.Arrows))
	}
}

func TestModule_Map_MinimalQuiver(t *testing.T) {
	yamlData := []byte(`
schema: "quiver@v0"
name: minimal
description: Minimal quiver
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if result.Name != "minimal" {
		t.Errorf("Name = %q, want minimal", result.Name)
	}
	if result.Description != "Minimal quiver" {
		t.Errorf("Description = %q", result.Description)
	}
}

func TestModule_Map_URL(t *testing.T) {
	yamlData := []byte(`
schema: "quiver@v0"
name: url-test
description: Test with URL
url: https://github.com/example/quiver
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if result.URL != "https://github.com/example/quiver" {
		t.Errorf("URL = %q", result.URL)
	}
}

func TestModule_Map_CompleteListing(t *testing.T) {
	yamlData := []byte(`
schema: "quiver@v0"
name: complete-listing
description: A complete quiver listing
url: https://example.com/quiver
maintainers:
  - alice
  - bob
tags:
  - testing
  - documentation
media:
  icon: https://example.com/icon.png
  banner: https://example.com/banner.png
arrows:
  - github.com/org1/arrow1
  - github.com/org2/arrow2
  - github.com/org3/arrow3
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(result.Maintainers) != 2 {
		t.Errorf("Maintainers count = %d, want 2", len(result.Maintainers))
	}
	if len(result.Tags) != 2 {
		t.Errorf("Tags count = %d, want 2", len(result.Tags))
	}
	if len(result.Arrows) != 3 {
		t.Errorf("Arrows count = %d, want 3", len(result.Arrows))
	}
}
