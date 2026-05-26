package models

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type ArrowView struct {
	Namespace domain.Namespace
	Metadata  domain.Arrow
	Versions  []VersionView
}
