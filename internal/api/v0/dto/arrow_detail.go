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

func ArrowDetailDTOFrom(a *arrow.ArrowDetailDTO) ArrowDetailDTO {
	return ArrowDetailDTO{
		Namespace:   string(a.Namespace),
		Name:        a.Manifest.Name,
		Version:     a.Manifest.Version,
		Description: a.Manifest.Description,
		License:     a.Manifest.License,
		State:       string(a.State),
		Tags:        a.Manifest.Tags,
		ActiveRun:   RunRecordDTOFrom(a.ActiveRun),
		LastReturn:  ReturnDTOFrom(a.LastReturn),
	}
}
