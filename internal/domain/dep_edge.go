package domain

type DepType string

const (
	ToolDep    DepType = "tool"
	ServiceDep DepType = "service"
)

// DependencyEdge represents a resolved dependency link from one arrow to another.
// Namespace carries the concrete resolved ref; Constraint preserves the original
// declared constraint so the update flow can re-evaluate upgrade eligibility.
type DependencyEdge struct {
	Namespace  Namespace
	Constraint string
	Type       DepType
}
