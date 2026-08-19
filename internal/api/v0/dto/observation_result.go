package dto

import (
	"fmt"
	"time"
)

// ObservationResult wraps one or more state snapshots.
type ObservationResult struct {
	// Items are the observed states (arrows, runtimes, or mixed).
	Items []ObservedItem `json:"items" yaml:"items"`
	// SnapshotTime is when this observation was captured (RFC3339 format).
	SnapshotTime string `json:"snapshot_time" yaml:"snapshot_time"`
}

// ObservedItem is a union, tagged by Kind.
type ObservedItem struct {
	// Kind is "arrow" or "runtime".
	Kind string `json:"kind" yaml:"kind"`
	// Arrow is present if Kind == "arrow".
	Arrow *ArrowStateDTO `json:"arrow,omitempty" yaml:"arrow,omitempty"`
	// Runtime is present if Kind == "runtime".
	Runtime *ArrowRuntimeDTO `json:"runtime,omitempty" yaml:"runtime,omitempty"`
}

// ArrowStateDTO is a lightweight arrow snapshot.
type ArrowStateDTO struct {
	Namespace string `json:"namespace" yaml:"namespace"`
	Name      string `json:"name" yaml:"name"`
	State     string `json:"state" yaml:"state"`
}

// CheckPayload validates the ObservationResult structure.
func (o *ObservationResult) CheckPayload() error {
	if o.Items == nil {
		return fmt.Errorf("observation result: items must not be nil")
	}

	// Validate timestamp is RFC3339 format
	if _, err := time.Parse(time.RFC3339, o.SnapshotTime); err != nil {
		return fmt.Errorf("observation result: snapshot_time must be RFC3339 format: %w", err)
	}

	return nil
}
