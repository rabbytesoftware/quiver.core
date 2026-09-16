package dto

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type ArrowReadmeDTO struct {
	Namespace string `json:"namespace"`
	Readme    string `json:"readme"`
}

func ArrowReadmeDTOFrom(ns domain.Namespace, readme string) *ArrowReadmeDTO {
	return &ArrowReadmeDTO{
		Namespace: string(ns),
		Readme:    readme,
	}
}
