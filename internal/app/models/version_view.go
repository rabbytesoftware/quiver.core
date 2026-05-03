package models

import "github.com/rabbytesoftware/quiver/internal/domain"

type VersionView struct {
	Namespace domain.Namespace
	Metadata  domain.Arrow
	State     domain.ArrowState
}
