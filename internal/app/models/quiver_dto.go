package models

import "github.com/rabbytesoftware/quiver/internal/domain"

type QuiverListDTO struct {
	Namespace   domain.Namespace `json:"namespace"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Tags        []string         `json:"tags"`
}

type QuiverDetailDTO struct {
	Namespace domain.Namespace      `json:"namespace"`
	Manifest  domain.QuiverManifest `json:"manifest"`
}
