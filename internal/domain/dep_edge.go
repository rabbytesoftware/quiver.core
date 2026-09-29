package domain

type DepType string

const (
	ToolDep    DepType = "tool"
	ServiceDep DepType = "service"
)

// DependencyEdge represents a declared dependency link from one arrow to
// another. Constraint is the declared selector; the dependency's identity is
// its namespace at that selector, never the ref the selector resolves to.
type DependencyEdge struct {
	Namespace  Namespace
	Constraint string
	Type       DepType
}
