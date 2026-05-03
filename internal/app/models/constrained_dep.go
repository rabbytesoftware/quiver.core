package models

import "github.com/rabbytesoftware/quiver/internal/domain"

type ConstrainedDep struct {
	Namespace     domain.Namespace
	OldConstraint string
	NewConstraint string
}
