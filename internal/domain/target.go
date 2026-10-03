package domain

import (
	"maps"
	"strings"
)

// ActivationRestart is the activation a method declares when what it staged
// only takes effect once the daemon restarts.
const ActivationRestart = "restart"

// Target is one OS's compiled recipe for an arrow.
type Target struct {
	Requirements Requirement       `json:"requirements"`
	Tools        []DependencyEdge  `json:"tools"`
	Services     []DependencyEdge  `json:"services"`
	Exports      map[string]string `json:"exports"`
	Lifecycle    TargetLifecycle   `json:"lifecycle"`
	Methods      map[string]Method `json:"methods"`
	Expose       Expose            `json:"expose"`
	// Activation maps a lifecycle method, by its manifest name, to what its
	// success leaves pending until it is applied.
	Activation map[string]string `json:"activation,omitempty"`
}

// ActivationMethods are the lifecycle methods, by manifest name, that may
// declare an activation: the ones whose success can leave something staged.
func ActivationMethods() []string {
	return []string{"update"}
}

// ActivationFor is the activation method (a MethodXxx constant) declares, or
// "" when its success takes effect at once.
func (t Target) ActivationFor(
	method string,
) string {
	return t.Activation[strings.TrimPrefix(method, "_")]
}

// MergeActivation overlays child onto parent, per method.
func MergeActivation(
	parent map[string]string,
	child map[string]string,
) map[string]string {
	if len(parent) == 0 && len(child) == 0 {
		return nil
	}
	merged := make(map[string]string, len(parent)+len(child))
	maps.Copy(merged, parent)
	maps.Copy(merged, child)
	return merged
}
