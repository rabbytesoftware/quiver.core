package models

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type ArrowListDTO struct {
	Namespace   domain.Namespace      `json:"namespace"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Tags        []string              `json:"tags"`
	Media       domain.ArrowMedia     `json:"media"`
	Versions    []InstalledVersionDTO `json:"versions"`
}
