package dto

import "github.com/rabbytesoftware/quiver.core/internal/app/models"

type ArrowDetailDTO struct {
	Namespace           string        `json:"namespace" yaml:"namespace"`
	Name                string        `json:"name" yaml:"name"`
	Description         string        `json:"description" yaml:"description"`
	License             string        `json:"license" yaml:"license"`
	State               string        `json:"state" yaml:"state"`
	Tags                []string      `json:"tags" yaml:"tags"`
	InstalledAt         string        `json:"installed_at,omitempty" yaml:"installed_at,omitempty"`
	InstalledConstraint string        `json:"installed_constraint,omitempty" yaml:"installed_constraint,omitempty"`
	UserInstalled       bool          `json:"user_installed" yaml:"user_installed"`
	ActiveRun           *RunRecordDTO `json:"active_run,omitempty" yaml:"active_run,omitempty"`
	LastReturn          *ReturnDTO    `json:"last_return,omitempty" yaml:"last_return,omitempty"`
}

func ArrowDetailDTOFrom(
	a *models.ArrowDetailDTO,
) ArrowDetailDTO {
	installedAt := ""
	if !a.InstalledAt.IsZero() {
		installedAt = a.InstalledAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return ArrowDetailDTO{
		Namespace:           string(a.Namespace),
		Name:                a.Name,
		Description:         a.Description,
		State:               string(a.State),
		Tags:                a.Tags,
		InstalledAt:         installedAt,
		InstalledConstraint: a.InstalledConstraint,
		UserInstalled:       a.UserInstalled,
		ActiveRun:           RunRecordDTOFrom(a.ActiveRun),
		LastReturn:          ReturnDTOFrom(a.LastReturn),
	}
}
