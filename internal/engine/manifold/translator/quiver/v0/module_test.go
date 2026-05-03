package v0_test

import (
	"fmt"
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
metadata:
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
metadata:
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
	if result.Arrows[0].Namespace != domain.Namespace("github.com/game/arrow1") {
		t.Errorf("Arrow 0 = %q", result.Arrows[0].Namespace)
	}
	if result.Arrows[1].Namespace != domain.Namespace("github.com/game/arrow2") {
		t.Errorf("Arrow 1 = %q", result.Arrows[1].Namespace)
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
metadata:
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
metadata:
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
metadata:
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
metadata:
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
metadata:
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
metadata:
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
metadata:
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
metadata:
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
metadata:
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
metadata:
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
metadata:
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

func TestModule_Map_ErrorPropagation(t *testing.T) {
	_, err := v0.Default.Map([]byte("invalid: [[["))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestModule_Map_LargeArrowList(t *testing.T) {
	arrowsYAML := "arrows:\n"
	for i := 0; i < 50; i++ {
		arrowsYAML += fmt.Sprintf("  - github.com/org/arrow%d\n", i)
	}
	yamlData := []byte(`
schema: "quiver@v0"
metadata:
  name: many-arrows
  description: Quiver with many arrows
` + arrowsYAML)

	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(result.Arrows) != 50 {
		t.Errorf("Arrows count = %d, want 50", len(result.Arrows))
	}
}

// Arrow entry tests

func TestArrowEntryV0_StringShorthand(t *testing.T) {
	input := []byte(`
schema: "quiver@v0"
metadata:
  name: "Test"
  description: "desc"
arrows:
  - github.com/valve/steamcmd
  - path: servers/cs2
  - namespace: github.com/other/tool
`)
	result, err := v0.Default.Map(input)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Arrows) != 3 {
		t.Fatalf("Arrows count = %d, want 3", len(result.Arrows))
	}
	if result.Arrows[0].Namespace != domain.Namespace("github.com/valve/steamcmd") {
		t.Errorf("Arrow 0 namespace = %q, want github.com/valve/steamcmd", result.Arrows[0].Namespace)
	}
	if result.Arrows[1].Namespace != domain.Namespace("servers/cs2") {
		t.Errorf("Arrow 1 namespace = %q, want servers/cs2", result.Arrows[1].Namespace)
	}
	if result.Arrows[2].Namespace != domain.Namespace("github.com/other/tool") {
		t.Errorf("Arrow 2 namespace = %q, want github.com/other/tool", result.Arrows[2].Namespace)
	}
}

func TestArrowEntryV0_PathForm(t *testing.T) {
	input := []byte(`
schema: "quiver@v0"
metadata:
  name: "Test"
  description: "desc"
arrows:
  - path: servers/cs2
`)
	result, err := v0.Default.Map(input)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(result.Arrows) != 1 {
		t.Fatalf("Arrows count = %d, want 1", len(result.Arrows))
	}
	if result.Arrows[0].Namespace != domain.Namespace("servers/cs2") {
		t.Errorf("Arrow namespace = %q, want servers/cs2", result.Arrows[0].Namespace)
	}
}

func TestArrowEntryV0_NamespaceForm(t *testing.T) {
	input := []byte(`
schema: "quiver@v0"
metadata:
  name: "Test"
  description: "desc"
arrows:
  - namespace: github.com/other/tool
`)
	result, err := v0.Default.Map(input)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(result.Arrows) != 1 {
		t.Fatalf("Arrows count = %d, want 1", len(result.Arrows))
	}
	if result.Arrows[0].Namespace != domain.Namespace("github.com/other/tool") {
		t.Errorf("Arrow namespace = %q, want github.com/other/tool", result.Arrows[0].Namespace)
	}
}

func TestArrowEntryV0_MixedArray(t *testing.T) {
	input := []byte(`
schema: "quiver@v0"
metadata:
  name: "Test"
  description: "desc"
arrows:
  - github.com/valve/steamcmd
  - path: servers/cs2
  - namespace: github.com/other/tool
`)
	result, err := v0.Default.Map(input)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(result.Arrows) != 3 {
		t.Fatalf("Arrows count = %d, want 3", len(result.Arrows))
	}
}

func TestArrowEntryV0_BothPathAndNamespace_Error(t *testing.T) {
	input := []byte(`
schema: "quiver@v0"
metadata:
  name: "Test"
  description: "desc"
arrows:
  - path: servers/cs2
    namespace: github.com/other/tool
`)
	_, err := v0.Default.Map(input)
	if err == nil {
		t.Fatal("expected error when both path and namespace are set")
	}
}

func TestArrowEntryV0_NeitherPathNorNamespace_Error(t *testing.T) {
	input := []byte(`
schema: "quiver@v0"
metadata:
  name: "Test"
  description: "desc"
arrows:
  - {}
`)
	_, err := v0.Default.Map(input)
	if err == nil {
		t.Fatal("expected error when neither path nor namespace is set")
	}
}

func TestArrowEntryV0_MetadataAllFields(t *testing.T) {
	input := []byte(`
schema: "quiver@v0"
metadata:
  name: full-meta
  version: "1.2.3"
  description: Full metadata test
  url: https://example.com
  maintainers:
    - alice
    - bob
  tags:
    - gaming
    - tools
  media:
    icon: https://example.com/icon.png
    banner: https://example.com/banner.png
`)
	result, err := v0.Default.Map(input)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if result.Name != "full-meta" {
		t.Errorf("Name = %q, want full-meta", result.Name)
	}
	if result.Version != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3", result.Version)
	}
	if result.Description != "Full metadata test" {
		t.Errorf("Description = %q", result.Description)
	}
	if result.URL != "https://example.com" {
		t.Errorf("URL = %q", result.URL)
	}
	if len(result.Maintainers) != 2 || result.Maintainers[0] != "alice" {
		t.Errorf("Maintainers = %v", result.Maintainers)
	}
	if len(result.Tags) != 2 || result.Tags[0] != "gaming" {
		t.Errorf("Tags = %v", result.Tags)
	}
	if result.Media.Icon != "https://example.com/icon.png" {
		t.Errorf("Icon = %q", result.Media.Icon)
	}
	if result.Media.Banner != "https://example.com/banner.png" {
		t.Errorf("Banner = %q", result.Media.Banner)
	}
}
