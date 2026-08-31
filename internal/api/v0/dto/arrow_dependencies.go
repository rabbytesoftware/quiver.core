package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type ArrowDependencyDTO struct {
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
}

type ArrowDependenciesDTO struct {
	Namespace    string               `json:"namespace"`
	Dependencies []ArrowDependencyDTO `json:"dependencies"`
}

func ArrowDependenciesDTOFrom(ns domain.Namespace, plan models.Plan) *ArrowDependenciesDTO {
	deps := make([]ArrowDependencyDTO, 0, len(plan))
	for _, entry := range plan {
		deps = append(deps, ArrowDependencyDTO{
			Namespace: string(entry.Namespace),
			Type:      string(entry.Type),
		})
	}
	return &ArrowDependenciesDTO{
		Namespace:    string(ns),
		Dependencies: deps,
	}
}
