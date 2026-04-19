package dto

import "github.com/rabbytesoftware/quiver/internal/app/arrow"

type ArrowDetailDTO struct {
	Namespace   string        `json:"namespace"`
	Name        string        `json:"name"`
	Version     string        `json:"version"`
	Description string        `json:"description"`
	License     string        `json:"license"`
	State       string        `json:"state"`
	Tags        []string      `json:"tags"`
	ActiveRun   *RunRecordDTO `json:"active_run,omitempty"`
	LastReturn  *ReturnDTO    `json:"last_return,omitempty"`
}

func ArrowDetailDTOFrom(
	a *arrow.ArrowDetailDTO,
) ArrowDetailDTO {
	return ArrowDetailDTO{
		Namespace:   string(a.Namespace),
		Name:        a.Name,
		Description: a.Description,
		State:       string(a.State),
		Tags:        a.Tags,
		ActiveRun:   RunRecordDTOFrom(a.ActiveRun),
		LastReturn:  ReturnDTOFrom(a.LastReturn),
	}
}
