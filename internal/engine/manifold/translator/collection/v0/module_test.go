package v0_test

import (
	"fmt"
	"testing"

	v0 "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator/collection/v0"
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
schema: "collection@v0"
metadata:
  name: test-quiver
  description: A test quiver
`)

func TestModule_Map_Minimal(t *testing.T) {
	manifest, _, err := v0.Default.Map(minimalQuiverYAML)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if manifest.Meta.Name != "test-quiver" {
		t.Errorf("Name = %q, want test-quiver", manifest.Meta.Name)
	}
	if manifest.Meta.Description != "A test quiver" {
		t.Errorf("Description = %q, want A test quiver", manifest.Meta.Description)
	}
}

var fullQuiverYAML = []byte(`
schema: "collection@v0"
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
	manifest, entries, err := v0.Default.Map(fullQuiverYAML)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if manifest.Meta.Name != "gaming-quiver" {
		t.Errorf("Name = %q, want gaming-quiver", manifest.Meta.Name)
	}
	if manifest.Meta.Description != "A collection of gaming tools" {
		t.Errorf("Description mismatch")
	}
	if manifest.Meta.URL != "https://gaming.example.com" {
		t.Errorf("URL = %q", manifest.Meta.URL)
	}

	if len(manifest.Meta.Maintainers) != 2 {
		t.Errorf("Maintainers count = %d, want 2", len(manifest.Meta.Maintainers))
	}
	if manifest.Meta.Maintainers[0] != "gaming-team" {
		t.Errorf("Maintainer 0 = %q", manifest.Meta.Maintainers[0])
	}

	if len(manifest.Meta.Tags) != 2 {
		t.Errorf("Tags count = %d, want 2", len(manifest.Meta.Tags))
	}

	if manifest.Meta.Media.Icon != "https://example.com/icon.png" {
		t.Errorf("Icon = %q", manifest.Meta.Media.Icon)
	}
	if manifest.Meta.Media.Banner != "https://example.com/banner.png" {
		t.Errorf("Banner = %q", manifest.Meta.Media.Banner)
	}

	if len(manifest.Arrows) != 0 {
		t.Errorf("Manifest.Arrows should be empty (entries not yet derived), got %d", len(manifest.Arrows))
	}

	if len(entries) != 2 {
		t.Fatalf("Entries count = %d, want 2", len(entries))
	}
	if entries[0].Namespace != "github.com/game/arrow1" {
		t.Errorf("Entry 0 namespace = %q, want github.com/game/arrow1", entries[0].Namespace)
	}
	if entries[1].Namespace != "github.com/game/arrow2" {
		t.Errorf("Entry 1 namespace = %q, want github.com/game/arrow2", entries[1].Namespace)
	}
}

func TestModule_Map_InvalidYAML(t *testing.T) {
	_, _, err := v0.Default.Map([]byte("not: valid: yaml: :"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestModule_Map_EmptyOptionalFields(t *testing.T) {
	yamlData := []byte(`
schema: "collection@v0"
metadata:
  name: minimal-quiver
  description: Minimal description
`)
	manifest, entries, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if manifest.Meta.URL != "" {
		t.Errorf("URL should be empty, got %q", manifest.Meta.URL)
	}
	if len(manifest.Meta.Maintainers) != 0 {
		t.Errorf("Maintainers should be empty")
	}
	if len(manifest.Meta.Tags) != 0 {
		t.Errorf("Tags should be empty")
	}
	if len(manifest.Arrows) != 0 {
		t.Errorf("Arrows should be empty")
	}
	if len(entries) != 0 {
		t.Errorf("Entries should be empty")
	}
}

func TestModule_Map_ArrowsWithSpecialChars(t *testing.T) {
	yamlData := []byte(`
schema: "collection@v0"
metadata:
  name: special-arrows
  description: Test
arrows:
  - github.com/org/arrow-with-dashes
  - github.com/org/arrow.with.dots
`)
	_, entries, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("Entries count = %d, want 2", len(entries))
	}
}

func TestModule_Map_MultipleArrows(t *testing.T) {
	yamlData := []byte(`
schema: "collection@v0"
metadata:
  name: multi-arrow
  description: Multiple arrows
arrows:
  - github.com/org/arrow1
  - github.com/org/arrow2
  - github.com/org/arrow3
`)
	_, entries, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("Entries count = %d, want 3", len(entries))
	}
}

func TestModule_Map_WithoutMedia(t *testing.T) {
	yamlData := []byte(`
schema: "collection@v0"
metadata:
  name: no-media
  description: Quiver without media
`)
	manifest, _, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if manifest.Meta.Media.Icon != "" {
		t.Errorf("Icon should be empty")
	}
	if manifest.Meta.Media.Banner != "" {
		t.Errorf("Banner should be empty")
	}
}

func TestModule_Map_WithMedia(t *testing.T) {
	yamlData := []byte(`
schema: "collection@v0"
metadata:
  name: with-media
  description: Quiver with media
  media:
    icon: https://example.com/icon.png
    banner: https://example.com/banner.png
`)
	manifest, _, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if manifest.Meta.Media.Icon != "https://example.com/icon.png" {
		t.Errorf("Icon = %q, want https://example.com/icon.png", manifest.Meta.Media.Icon)
	}
	if manifest.Meta.Media.Banner != "https://example.com/banner.png" {
		t.Errorf("Banner = %q, want https://example.com/banner.png", manifest.Meta.Media.Banner)
	}
}

func TestModule_Map_WithMaintainers(t *testing.T) {
	yamlData := []byte(`
schema: "collection@v0"
metadata:
  name: with-maintainers
  description: Quiver with maintainers
  maintainers:
    - alice
    - bob
`)
	manifest, _, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(manifest.Meta.Maintainers) != 2 {
		t.Errorf("Maintainers count = %d, want 2", len(manifest.Meta.Maintainers))
	}
	if manifest.Meta.Maintainers[0] != "alice" {
		t.Errorf("First maintainer = %q, want alice", manifest.Meta.Maintainers[0])
	}
}

func TestModule_Map_WithTags(t *testing.T) {
	yamlData := []byte(`
schema: "collection@v0"
metadata:
  name: with-tags
  description: Quiver with tags
  tags:
    - backend
    - database
    - cache
`)
	manifest, _, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(manifest.Meta.Tags) != 3 {
		t.Errorf("Tags count = %d, want 3", len(manifest.Meta.Tags))
	}
}

func TestModule_Map_AllFields(t *testing.T) {
	yamlData := []byte(`
schema: "collection@v0"
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
	manifest, entries, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if manifest.Meta.Name != "full-quiver" {
		t.Errorf("Name = %q, want full-quiver", manifest.Meta.Name)
	}
	if manifest.Meta.Description != "A complete quiver" {
		t.Errorf("Description = %q", manifest.Meta.Description)
	}
	if manifest.Meta.URL != "https://example.com" {
		t.Errorf("URL = %q", manifest.Meta.URL)
	}
	if len(manifest.Meta.Maintainers) != 1 {
		t.Errorf("Maintainers count = %d", len(manifest.Meta.Maintainers))
	}
	if len(manifest.Meta.Tags) != 1 {
		t.Errorf("Tags count = %d", len(manifest.Meta.Tags))
	}
	if len(manifest.Arrows) != 0 {
		t.Errorf("Manifest.Arrows should be empty (entries not yet derived)")
	}
	if len(entries) != 2 {
		t.Errorf("Entries count = %d, want 2", len(entries))
	}
}

func TestModule_Map_MinimalQuiver(t *testing.T) {
	yamlData := []byte(`
schema: "collection@v0"
metadata:
  name: minimal
  description: Minimal quiver
`)
	manifest, _, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if manifest.Meta.Name != "minimal" {
		t.Errorf("Name = %q, want minimal", manifest.Meta.Name)
	}
	if manifest.Meta.Description != "Minimal quiver" {
		t.Errorf("Description = %q", manifest.Meta.Description)
	}
}

func TestModule_Map_URL(t *testing.T) {
	yamlData := []byte(`
schema: "collection@v0"
metadata:
  name: url-test
  description: Test with URL
  url: https://github.com/example/quiver
`)
	manifest, _, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if manifest.Meta.URL != "https://github.com/example/quiver" {
		t.Errorf("URL = %q", manifest.Meta.URL)
	}
}

func TestModule_Map_CompleteListing(t *testing.T) {
	yamlData := []byte(`
schema: "collection@v0"
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
	manifest, entries, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(manifest.Meta.Maintainers) != 2 {
		t.Errorf("Maintainers count = %d, want 2", len(manifest.Meta.Maintainers))
	}
	if len(manifest.Meta.Tags) != 2 {
		t.Errorf("Tags count = %d, want 2", len(manifest.Meta.Tags))
	}
	if len(entries) != 3 {
		t.Errorf("Entries count = %d, want 3", len(entries))
	}
}

func TestModule_Map_ErrorPropagation(t *testing.T) {
	_, _, err := v0.Default.Map([]byte("invalid: [[["))
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
schema: "collection@v0"
metadata:
  name: many-arrows
  description: Quiver with many arrows
` + arrowsYAML)

	_, entries, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(entries) != 50 {
		t.Errorf("Entries count = %d, want 50", len(entries))
	}
}

// Arrow entry tests

func TestArrowEntryV0_StringShorthand(t *testing.T) {
	input := []byte(`
schema: "collection@v0"
metadata:
  name: "Test"
  description: "desc"
arrows:
  - github.com/valve/steamcmd
  - path: servers/cs2
  - namespace: github.com/other/tool
`)
	_, entries, err := v0.Default.Map(input)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("Entries count = %d, want 3", len(entries))
	}
	if entries[0].Namespace != "github.com/valve/steamcmd" {
		t.Errorf("Entry 0 namespace = %q, want github.com/valve/steamcmd", entries[0].Namespace)
	}
	if entries[1].Path != "servers/cs2" {
		t.Errorf("Entry 1 path = %q, want servers/cs2", entries[1].Path)
	}
	if entries[2].Namespace != "github.com/other/tool" {
		t.Errorf("Entry 2 namespace = %q, want github.com/other/tool", entries[2].Namespace)
	}
}

func TestArrowEntryV0_PathForm(t *testing.T) {
	input := []byte(`
schema: "collection@v0"
metadata:
  name: "Test"
  description: "desc"
arrows:
  - path: servers/cs2
`)
	_, entries, err := v0.Default.Map(input)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Entries count = %d, want 1", len(entries))
	}
	if entries[0].Path != "servers/cs2" {
		t.Errorf("Entry path = %q, want servers/cs2", entries[0].Path)
	}
}

func TestArrowEntryV0_NamespaceForm(t *testing.T) {
	input := []byte(`
schema: "collection@v0"
metadata:
  name: "Test"
  description: "desc"
arrows:
  - namespace: github.com/other/tool
`)
	_, entries, err := v0.Default.Map(input)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Entries count = %d, want 1", len(entries))
	}
	if entries[0].Namespace != "github.com/other/tool" {
		t.Errorf("Entry namespace = %q, want github.com/other/tool", entries[0].Namespace)
	}
}

func TestArrowEntryV0_MixedArray(t *testing.T) {
	input := []byte(`
schema: "collection@v0"
metadata:
  name: "Test"
  description: "desc"
arrows:
  - github.com/valve/steamcmd
  - path: servers/cs2
  - namespace: github.com/other/tool
`)
	_, entries, err := v0.Default.Map(input)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("Entries count = %d, want 3", len(entries))
	}
}

// The authored `version` here is deliberate: a legacy manifest carrying one must
// still land every other metadata field, not lose the block to one stray key.
func TestArrowEntryV0_MetadataAllFields(t *testing.T) {
	input := []byte(`
schema: "collection@v0"
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
	manifest, _, err := v0.Default.Map(input)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if manifest.Meta.Name != "full-meta" {
		t.Errorf("Name = %q, want full-meta", manifest.Meta.Name)
	}
	if manifest.Meta.Description != "Full metadata test" {
		t.Errorf("Description = %q", manifest.Meta.Description)
	}
	if manifest.Meta.URL != "https://example.com" {
		t.Errorf("URL = %q", manifest.Meta.URL)
	}
	if len(manifest.Meta.Maintainers) != 2 || manifest.Meta.Maintainers[0] != "alice" {
		t.Errorf("Maintainers = %v", manifest.Meta.Maintainers)
	}
	if len(manifest.Meta.Tags) != 2 || manifest.Meta.Tags[0] != "gaming" {
		t.Errorf("Tags = %v", manifest.Meta.Tags)
	}
	if manifest.Meta.Media.Icon != "https://example.com/icon.png" {
		t.Errorf("Icon = %q", manifest.Meta.Media.Icon)
	}
	if manifest.Meta.Media.Banner != "https://example.com/banner.png" {
		t.Errorf("Banner = %q", manifest.Meta.Media.Banner)
	}
}

// The schema still declares a version property purely so the key is tolerated —
// metadata sets additionalProperties:false, so dropping it would turn a stray
// version into a hard validation error. Neither metadataV0 nor CollectionMeta has
// a matching field, so the authored value has nowhere to land: a collection is a
// list of arrows that each carry their own ref, and the list names no revision of
// its own. This pins that such a manifest parses clean.
func TestMap_MetadataVersion_IsToleratedAndIgnored(t *testing.T) {
	input := []byte(`
schema: "collection@v0"
metadata:
  name: legacy
  version: "9.9.9"
  description: Authored before the version field was retired
arrows:
  - namespace: github.com/valve/steamcmd
`)
	manifest, entries, err := v0.Default.Map(input)
	if err != nil {
		t.Fatalf("Map() error = %v, want nil: a manifest carrying metadata.version must parse, not be rejected", err)
	}
	if manifest.Meta.Name != "legacy" {
		t.Errorf("Name = %q, want legacy", manifest.Meta.Name)
	}
	if manifest.Meta.Description != "Authored before the version field was retired" {
		t.Errorf("Description = %q", manifest.Meta.Description)
	}
	if len(entries) != 1 || entries[0].Namespace != "github.com/valve/steamcmd" {
		t.Errorf("Entries = %v, want the one authored member", entries)
	}
}

// An entry that is neither a scalar nor a path/namespace mapping — here a `path`
// holding a sequence — must surface the decode error rather than silently
// producing an entry with an empty path that would later derive a bad namespace.
func TestArrowEntryV0_UnmarshalYAML_UndecodableMapping(t *testing.T) {
	input := []byte(`
schema: "collection@v0"
metadata:
  name: bad-entry
  description: entry that cannot decode
arrows:
  - path:
      - servers
      - cs2
`)
	if _, _, err := v0.Default.Map(input); err == nil {
		t.Fatal("Map() error = nil, want an error for an entry whose path is not a string")
	}
}
