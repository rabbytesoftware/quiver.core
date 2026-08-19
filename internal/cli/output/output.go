// Package output holds the payload shapes the CLI emits under --output.
//
// It carries only what the API DTOs do not already express. A command whose
// result is an API resource returns that resource's DTO unchanged: an
// observation is an ArrowRuntimeDTO, a lifecycle run's steps are
// StepProgressDTOs, an arrow's detail view is an ArrowDetailDTO. Restating
// those here under new field names would put two shapes for one concept in the
// binary, which is the inconsistency this package exists to remove.
//
// Payloads carry no success flag. A command that fails reports it through
// CommandModel.Err, and tui.Runner returns that error without writing a
// payload at all, so a rendered payload is by construction a successful one.
package output

import (
	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
)

// Action is a mutation's verb.
type Action string

// The mutations. Each names the operation a command performs, not the HTTP
// method that carries it. ActionUse is local-only: it switches the active
// context and never reaches the daemon.
const (
	ActionAdd      Action = "add"
	ActionRemove   Action = "remove"
	ActionRefresh  Action = "refresh"
	ActionFollow   Action = "follow"
	ActionUnfollow Action = "unfollow"
	ActionUpdate   Action = "update"
	ActionUse      Action = "use"
)

// Past returns the verb's past tense, for the one-line table rendering
// ("added github.com/u/r"). The forms are spelled out rather than derived:
// suffix arithmetic on English verbs is right only by accident.
func (a Action) Past() string {
	switch a {
	case ActionAdd:
		return "added"
	case ActionRemove:
		return "removed"
	case ActionRefresh:
		return "refreshed"
	case ActionFollow:
		return "followed"
	case ActionUnfollow:
		return "unfollowed"
	case ActionUpdate:
		return "updated"
	case ActionUse:
		return "switched to"
	}

	return string(a)
}

// Mutation is the payload of a command that changes one thing and reports the
// outcome: the arrow and collection catalog operations, and the local context
// edits.
//
// These commands previously wrote a bare line to stdout and produced nothing
// at all under -o json, which is the gap this type closes.
type Mutation struct {
	// Action is the operation performed.
	Action Action `json:"action" yaml:"action"`
	// Subject is the namespace the operation addressed.
	Subject string `json:"subject" yaml:"subject"`
	// At is when the mutation completed, in RFC3339.
	At string `json:"at" yaml:"at"`
}

// Catalog is the payload of the discovery commands, list and search.
//
// Arrows and collections stay in separate homogeneous lists so a consumer can
// reach either directly (`jq '.arrows[]'`). Folding them into one list tagged
// by kind would put every field behind a nullable branch.
type Catalog struct {
	// Arrows are the matching arrows, never nil.
	Arrows []ArrowRow `json:"arrows" yaml:"arrows"`
	// Collections are the matching collections, never nil.
	Collections []CollectionRow `json:"collections" yaml:"collections"`
	// Total is len(Arrows) + len(Collections).
	Total int `json:"total" yaml:"total"`
	// Query is the pattern that filtered the catalog, absent when unfiltered.
	Query string `json:"query,omitempty" yaml:"query,omitempty"`
}

// ArrowRow is one arrow in a Catalog.
//
// Ref is the ref the arrow was registered under, which is the handle
// `arrow remove` expects — dropping it from the listing would leave no way to
// discover the argument the removal needs.
type ArrowRow struct {
	Namespace string `json:"namespace" yaml:"namespace"`
	Name      string `json:"name" yaml:"name"`
	Ref       string `json:"ref" yaml:"ref"`
	State     string `json:"state" yaml:"state"`
}

// CollectionRow is one collection in a Catalog.
type CollectionRow struct {
	Namespace string `json:"namespace" yaml:"namespace"`
	Name      string `json:"name" yaml:"name"`
	Arrows    int    `json:"arrows" yaml:"arrows"`
}

// NewCatalog returns a Catalog with Total derived and both lists non-nil, so
// the encoders emit [] rather than null for an empty result.
func NewCatalog(arrows []ArrowRow, collections []CollectionRow, query string) Catalog {
	if arrows == nil {
		arrows = []ArrowRow{}
	}

	if collections == nil {
		collections = []CollectionRow{}
	}

	return Catalog{
		Arrows:      arrows,
		Collections: collections,
		Total:       len(arrows) + len(collections),
		Query:       query,
	}
}

// Run is the terminal result of a lifecycle method: install, run, stop,
// update, uninstall, or a custom method from the manifest.
//
// Steps reuses the API's StepProgressDTO rather than restating it. The daemon
// already has a shape for a step and a second one here would be two
// vocabularies for one concept.
type Run struct {
	// Subject is the namespace the method ran against.
	Subject string `json:"subject" yaml:"subject"`
	// Method is the method invoked, under the name the user typed.
	Method string `json:"method" yaml:"method"`
	// Outcome is the daemon's verdict: success, failed, or cancelled.
	Outcome string `json:"outcome" yaml:"outcome"`
	// State is the arrow's state once the method finished.
	State string `json:"state" yaml:"state"`
	// Steps are the steps the run executed, in order.
	Steps []dto.StepProgressDTO `json:"steps" yaml:"steps"`
	// Error is the failing step's message, absent on success.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`
	// At is when the run finished, in RFC3339.
	At string `json:"at" yaml:"at"`
}

// OK reports whether the run succeeded.
func (r Run) OK() bool { return r.Outcome == "success" }

// NoOp is the payload of a method the daemon completed as an idempotent
// no-op — an install of an already-installed arrow, say. It is a distinct
// shape from Run because no run happened: there are no steps and no state
// transition to report, and saying so is the whole point.
type NoOp struct {
	Subject string `json:"subject" yaml:"subject"`
	Method  string `json:"method" yaml:"method"`
	Reason  string `json:"reason" yaml:"reason"`
	At      string `json:"at" yaml:"at"`
}
