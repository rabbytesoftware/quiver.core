package storage

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type VersionRef struct {
	Namespace domain.Namespace
	Metadata  domain.Arrow
}
