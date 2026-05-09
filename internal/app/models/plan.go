package models

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type Plan []PlanEntry

type PlanEntry struct {
	Namespace domain.Namespace
	Type      domain.DepType
}
