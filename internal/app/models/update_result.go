package models

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type UpdateResult struct {
	AddedDeps           []domain.Namespace
	RemovedFromManifest []domain.Namespace
	SafeToUninstall     []domain.Namespace
	ConstrainedDeps     []ConstrainedDep
	NewRef              string
}
