package models

import "github.com/rabbytesoftware/quiver/internal/domain"

type DepDiff struct {
	Added       []domain.DependencyEdge
	Removed     []domain.DependencyEdge
	Constrained []ConstrainedDep
}
