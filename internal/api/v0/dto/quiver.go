package dto

import "github.com/rabbytesoftware/quiver/internal/domain"

type QuiverDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

func QuiverDTOFrom(q domain.Quiver) QuiverDTO {
	return QuiverDTO{
		Namespace: string(q.Namespace),
	}
}
