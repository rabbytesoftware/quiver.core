package models

import "github.com/rabbytesoftware/quiver/internal/domain"

type ArrowView struct {
	Namespace domain.Namespace
	Metadata  domain.Arrow
	Versions  []VersionView
}
