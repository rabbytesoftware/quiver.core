package models

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type DepDiff struct {
	Added       []domain.DependencyEdge
	Removed     []domain.DependencyEdge
	Constrained []ConstrainedDep
}
