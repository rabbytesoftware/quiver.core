package models

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type UpdateResult struct {
	AddedDeps           []domain.Namespace
	RemovedFromManifest []domain.Namespace
	SafeToUninstall     []domain.Namespace
	ConstrainedDeps     []ConstrainedDep
	// Available is what the update found ahead of an installed row, which
	// only the runtime update moves it to; nil when current.
	Available *domain.Available
}
