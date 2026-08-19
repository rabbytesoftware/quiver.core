package dto

import (
	"fmt"
	"time"
)

// LifecycleOutcome wraps the result of a lifecycle operation.
type LifecycleOutcome struct {
	// Subject is the namespace being operated on.
	Subject string `json:"subject" yaml:"subject"`
	// Action is the operation performed (install, run, stop, update, uninstall).
	Action string `json:"action" yaml:"action"`
	// Success is true if the operation completed without error.
	Success bool `json:"success" yaml:"success"`
	// Steps are the execution steps and their outcomes.
	Steps []StepRecord `json:"steps" yaml:"steps"`
	// FinalState is the arrow's state after completion.
	FinalState string `json:"final_state" yaml:"final_state"`
	// Timestamp is when the operation completed (RFC3339 format).
	Timestamp string `json:"timestamp" yaml:"timestamp"`
}

// StepRecord is a single step's name, state, and duration.
type StepRecord struct {
	// Name is the step's display name.
	Name string `json:"name" yaml:"name"`
	// State is the step's outcome (pending, running, done, failed).
	State string `json:"state" yaml:"state"`
	// Duration is how long the step took (e.g., "3.2s").
	Duration string `json:"duration" yaml:"duration"`
	// Error is present if the step failed.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`
}

// CheckPayload validates the LifecycleOutcome structure.
func (l *LifecycleOutcome) CheckPayload() error {
	validActions := map[string]bool{
		"install":   true,
		"run":       true,
		"stop":      true,
		"update":    true,
		"uninstall": true,
	}

	if !validActions[l.Action] {
		return fmt.Errorf("lifecycle outcome: action must be one of (install, run, stop, update, uninstall), got %q", l.Action)
	}

	if l.Steps == nil {
		return fmt.Errorf("lifecycle outcome: steps must not be nil")
	}

	if _, err := time.Parse(time.RFC3339, l.Timestamp); err != nil {
		return fmt.Errorf("lifecycle outcome: timestamp must be RFC3339 format: %w", err)
	}

	return nil
}
