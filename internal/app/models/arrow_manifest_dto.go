package models

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type ArrowManifestDTO struct {
	Namespace   domain.Namespace            `json:"namespace"`
	Name        string                      `json:"name"`
	Description string                      `json:"description"`
	Version     string                      `json:"version"`
	Tags        []string                    `json:"tags"`
	Variables   []domain.Variable           `json:"variables"`
	Targets     map[domain.OS]domain.Target `json:"targets"`
	Manifest    *domain.Arrow               `json:"manifest"`
}
