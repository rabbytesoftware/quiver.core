package storage

import "github.com/rabbytesoftware/quiver/internal/domain"

type VersionRef struct {
	Namespace domain.Namespace
	Metadata  domain.Arrow
}
