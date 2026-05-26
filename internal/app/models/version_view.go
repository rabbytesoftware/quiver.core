package models

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type VersionView struct {
	Namespace domain.Namespace
	Metadata  domain.Arrow
	State     domain.ArrowState
}
