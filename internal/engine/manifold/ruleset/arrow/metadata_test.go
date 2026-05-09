package arrow

import (
	"strings"
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
)

func TestMetadataRule_Name(t *testing.T) {
	rule := MetadataRule{}
	if rule.Name() != "metadata" {
		t.Errorf("Name() = %q, want metadata", rule.Name())
	}
}

func TestMetadataRule_ValidMetadata(t *testing.T) {
	rule := MetadataRule{}
	m := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:        "my-arrow",
			Version:     "1.0.0",
			Description: "A test arrow",
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestMetadataRule_MissingName(t *testing.T) {
	rule := MetadataRule{}
	m := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Version:     "1.0.0",
			Description: "A test arrow",
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) == 0 {
		t.Fatal("expected error for missing name")
	}
	if errs[0].Rule != "required" {
		t.Errorf("expected rule %q, got %q", "required", errs[0].Rule)
	}
	if !strings.Contains(errs[0].Field, "metadata.name") {
		t.Errorf("expected field to contain 'metadata.name', got %q", errs[0].Field)
	}
}

func TestMetadataRule_NameTooLong(t *testing.T) {
	rule := MetadataRule{}
	longName := strings.Repeat("a", domain.MaxNameLength+1)
	m := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:        longName,
			Version:     "1.0.0",
			Description: "A test arrow",
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) == 0 {
		t.Fatal("expected error for name exceeding max length")
	}
	if errs[0].Rule != "max_length" {
		t.Errorf("expected rule %q, got %q", "max_length", errs[0].Rule)
	}
}

func TestMetadataRule_NameAtMaxLength(t *testing.T) {
	rule := MetadataRule{}
	maxName := strings.Repeat("a", domain.MaxNameLength)
	m := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:        maxName,
			Version:     "1.0.0",
			Description: "A test arrow",
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) != 0 {
		t.Fatalf("expected no errors for name at max length, got: %v", errs)
	}
}

func TestMetadataRule_DescriptionTooLong(t *testing.T) {
	rule := MetadataRule{}
	longDescription := strings.Repeat("a", domain.MaxDescriptionLength+1)
	m := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:        "my-arrow",
			Version:     "1.0.0",
			Description: longDescription,
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) == 0 {
		t.Fatal("expected error for description exceeding max length")
	}
	if errs[0].Rule != "max_length" {
		t.Errorf("expected rule %q, got %q", "max_length", errs[0].Rule)
	}
}

func TestMetadataRule_DescriptionAtMaxLength(t *testing.T) {
	rule := MetadataRule{}
	maxDescription := strings.Repeat("a", domain.MaxDescriptionLength)
	m := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:        "my-arrow",
			Version:     "1.0.0",
			Description: maxDescription,
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) != 0 {
		t.Fatalf("expected no errors for description at max length, got: %v", errs)
	}
}

func TestMetadataRule_EmptyDescription(t *testing.T) {
	rule := MetadataRule{}
	m := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:        "my-arrow",
			Version:     "1.0.0",
			Description: "",
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty description (optional), got: %v", errs)
	}
}

func TestMetadataRule_MultipleErrors(t *testing.T) {
	rule := MetadataRule{}
	longName := strings.Repeat("b", domain.MaxNameLength+1)
	longDescription := strings.Repeat("c", domain.MaxDescriptionLength+1)
	m := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:        longName,
			Version:     "1.0.0",
			Description: longDescription,
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 errors, got %d: %v", len(errs), errs)
	}
	hasNameErr := false
	hasDescErr := false
	for _, err := range errs {
		if strings.Contains(err.Field, "name") {
			hasNameErr = true
		}
		if strings.Contains(err.Field, "description") {
			hasDescErr = true
		}
	}
	if !hasNameErr || !hasDescErr {
		t.Error("expected both name and description errors")
	}
}

func TestMetadataRule_SpecialCharactersInName(t *testing.T) {
	rule := MetadataRule{}
	m := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:        "my-arrow_v1.0",
			Version:     "1.0.0",
			Description: "A test arrow",
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) != 0 {
		t.Fatalf("expected no errors for special characters in name, got: %v", errs)
	}
}

func TestMetadataRule_UnicodeInDescription(t *testing.T) {
	rule := MetadataRule{}
	m := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:        "my-arrow",
			Version:     "1.0.0",
			Description: "A test arrow with émojis and spëcial chars",
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) != 0 {
		t.Fatalf("expected no errors for unicode characters, got: %v", errs)
	}
}
