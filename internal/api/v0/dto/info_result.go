package dto

import "fmt"

// InfoResult wraps detailed information about a subject.
type InfoResult struct {
	// Kind identifies what kind of subject (arrow, collection, context).
	Kind string `json:"kind" yaml:"kind"`
	// Subject is the detailed information (arrow: ArrowDetailDTO, etc.).
	Subject any `json:"subject" yaml:"subject"`
	// RelatedInfo is optional context (e.g., methods available, dependents).
	RelatedInfo map[string]any `json:"related_info,omitempty" yaml:"related_info,omitempty"`
}

// CheckPayload validates the InfoResult structure.
func (i *InfoResult) CheckPayload() error {
	validKinds := map[string]bool{
		"arrow":      true,
		"collection": true,
		"context":    true,
	}

	if !validKinds[i.Kind] {
		return fmt.Errorf("info result: kind must be one of (arrow, collection, context), got %q", i.Kind)
	}

	if i.Subject == nil {
		return fmt.Errorf("info result: subject must not be nil")
	}

	return nil
}
