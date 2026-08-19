package dto

import (
	"fmt"
	"time"
)

// MutationResult wraps the outcome of a catalog mutation.
type MutationResult struct {
	// Action is the operation performed (add, remove, refresh, follow, unfollow).
	Action string `json:"action" yaml:"action"`
	// Subject is the namespace or context name being mutated.
	Subject string `json:"subject" yaml:"subject"`
	// Success is true if the mutation succeeded.
	Success bool `json:"success" yaml:"success"`
	// Message is a human-readable summary (or error text if success=false).
	Message string `json:"message" yaml:"message"`
	// Timestamp is when the mutation was completed (RFC3339 format).
	Timestamp string `json:"timestamp" yaml:"timestamp"`
}

// CheckPayload validates the MutationResult structure.
func (m *MutationResult) CheckPayload() error {
	validActions := map[string]bool{
		"add":      true,
		"remove":   true,
		"refresh":  true,
		"follow":   true,
		"unfollow": true,
	}

	if !validActions[m.Action] {
		return fmt.Errorf("mutation result: action must be one of (add, remove, refresh, follow, unfollow), got %q", m.Action)
	}

	if _, err := time.Parse(time.RFC3339, m.Timestamp); err != nil {
		return fmt.Errorf("mutation result: timestamp must be RFC3339 format: %w", err)
	}

	return nil
}
