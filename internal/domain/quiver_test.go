package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestQuiver_Structure(t *testing.T) {
	// Test that Quiver struct can be created and has expected fields
	quiver := Quiver{
		ID:          "test-quiver-123",
		Name:        "Test Quiver",
		Description: "A test quiver for unit testing",
		Banner:      URL("https://example.com/banner.png"),
		URL:         URL("https://example.com/quiver"),
		Security:    Security("trusted"),
		Maintainers: []string{"test@example.com"},
		Version:     "1.0.0",
		InstalledArrows: []Arrow{
			{
				ID:          uuid.New(),
				Name:        "Test Arrow",
				Description: "A test arrow",
				Version:     "1.0.0",
			},
		},
		ListedArrows: []Namespace{
			Namespace("github.com/test/namespace"),
		},
	}

	// Test field access
	if quiver.ID != "test-quiver-123" {
		t.Errorf("Expected ID 'test-quiver-123', got %q", quiver.ID)
	}

	if quiver.Name != "Test Quiver" {
		t.Errorf("Expected Name 'Test Quiver', got %q", quiver.Name)
	}

	if quiver.Description != "A test quiver for unit testing" {
		t.Errorf("Expected Description 'A test quiver for unit testing', got %q", quiver.Description)
	}

	if quiver.Version != "1.0.0" {
		t.Errorf("Expected Version '1.0.0', got %q", quiver.Version)
	}

	if len(quiver.Maintainers) != 1 || quiver.Maintainers[0] != "test@example.com" {
		t.Errorf("Expected Maintainers ['test@example.com'], got %v", quiver.Maintainers)
	}

	if len(quiver.InstalledArrows) != 1 {
		t.Errorf("Expected 1 installed arrow, got %d", len(quiver.InstalledArrows))
	}

	if len(quiver.ListedArrows) != 1 {
		t.Errorf("Expected 1 listed arrow, got %d", len(quiver.ListedArrows))
	}
}

func TestQuiver_EmptyQuiver(t *testing.T) {
	// Test empty quiver
	quiver := Quiver{}

	if quiver.ID != "" {
		t.Errorf("Expected empty ID, got %q", quiver.ID)
	}

	if quiver.Name != "" {
		t.Errorf("Expected empty Name, got %q", quiver.Name)
	}

	if quiver.InstalledArrows != nil {
		t.Errorf("Expected nil InstalledArrows, got %v", quiver.InstalledArrows)
	}

	if quiver.ListedArrows != nil {
		t.Errorf("Expected nil ListedArrows, got %v", quiver.ListedArrows)
	}

	if quiver.Maintainers != nil {
		t.Errorf("Expected nil Maintainers, got %v", quiver.Maintainers)
	}
}

func TestQuiver_SystemTypes(t *testing.T) {
	// Test that system types work correctly
	quiver := Quiver{
		Banner:   URL("https://example.com/banner.png"),
		URL:      URL("https://example.com"),
		Security: Security("trusted"),
	}

	// Test URL methods
	if !quiver.Banner.IsValid() {
		t.Error("Expected Banner URL to be valid")
	}

	if !quiver.URL.IsValid() {
		t.Error("Expected URL to be valid")
	}

	// Test Security methods
	if !quiver.Security.IsTrusted() {
		t.Error("Expected Security to be trusted")
	}
}

func TestQuiver_ArrowTypes(t *testing.T) {
	// Test arrow types
	arrow1 := Arrow{
		ID:          uuid.New(),
		Name:        "Test Arrow",
		Description: "Test description",
		Version:     "1.0.0",
	}

	namespace1 := Namespace("github.com/test/namespace")

	quiver := Quiver{
		InstalledArrows: []Arrow{arrow1},
		ListedArrows:    []Namespace{namespace1},
	}

	if len(quiver.InstalledArrows) != 1 {
		t.Errorf("Expected 1 installed arrow, got %d", len(quiver.InstalledArrows))
	}

	if quiver.InstalledArrows[0].ID == uuid.Nil {
		t.Error("Expected arrow ID to not be nil")
	}

	if len(quiver.ListedArrows) != 1 {
		t.Errorf("Expected 1 listed arrow, got %d", len(quiver.ListedArrows))
	}

	if quiver.ListedArrows[0].Validate() != nil {
		t.Error("Expected arrow namespace to be valid")
	}
}
