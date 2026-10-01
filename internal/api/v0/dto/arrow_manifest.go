package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
)

type ArrowManifestDTO struct {
	Namespace   string                      `json:"namespace" yaml:"namespace"`
	Name        string                      `json:"name" yaml:"name"`
	Description string                      `json:"description" yaml:"description"`
	Tags        []string                    `json:"tags" yaml:"tags"`
	Variables   []domain.Variable           `json:"variables" yaml:"variables"`
	Targets     map[domain.OS]domain.Target `json:"targets" yaml:"targets"`
	Manifest    *ManifestContentDTO         `json:"manifest" yaml:"manifest"`
}

// ManifestContentDTO is the manifest as its author wrote it. What Quiver
// records about an installed row (selector, resolved and available refs,
// install and use stamps) belongs to the arrow detail, never here.
type ManifestContentDTO struct {
	Metadata  domain.ArrowMeta            `json:"metadata" yaml:"metadata"`
	Variables []domain.Variable           `json:"variables" yaml:"variables"`
	Netbridge []netbridge.PortDef         `json:"netbridge" yaml:"netbridge"`
	Targets   map[domain.OS]domain.Target `json:"targets" yaml:"targets"`
	Readme    string                      `json:"readme,omitempty" yaml:"readme,omitempty"`
}

func ArrowManifestDTOFrom(a *models.ArrowManifestDTO) *ArrowManifestDTO {
	if a == nil {
		return nil
	}
	return &ArrowManifestDTO{
		Namespace:   string(a.Namespace),
		Name:        a.Name,
		Description: a.Description,
		Tags:        a.Tags,
		Variables:   a.Variables,
		Targets:     a.Targets,
		Manifest:    manifestContentFrom(a.Manifest),
	}
}

func manifestContentFrom(a *domain.Arrow) *ManifestContentDTO {
	if a == nil {
		return nil
	}
	return &ManifestContentDTO{
		Metadata:  a.ArrowMeta,
		Variables: a.Variables,
		Netbridge: a.Netbridge,
		Targets:   a.Targets,
		Readme:    a.Readme,
	}
}
