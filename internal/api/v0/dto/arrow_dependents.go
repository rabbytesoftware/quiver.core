package dto

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type ArrowDependentsDTO struct {
	Namespace  string   `json:"namespace"`
	Dependents []string `json:"dependents"`
}

func ArrowDependentsDTOFrom(ns domain.Namespace, dependents []domain.Namespace) *ArrowDependentsDTO {
	result := make([]string, 0, len(dependents))
	for _, d := range dependents {
		result = append(result, string(d))
	}
	return &ArrowDependentsDTO{
		Namespace:  string(ns),
		Dependents: result,
	}
}
