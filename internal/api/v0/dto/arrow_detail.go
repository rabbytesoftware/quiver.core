package dto

import "github.com/rabbytesoftware/quiver.core/internal/app/models"

type ArrowDetailDTO struct {
	Namespace     string   `json:"namespace" yaml:"namespace"`
	Name          string   `json:"name" yaml:"name"`
	Description   string   `json:"description" yaml:"description"`
	License       string   `json:"license" yaml:"license"`
	State         string   `json:"state" yaml:"state"`
	Tags          []string `json:"tags" yaml:"tags"`
	InstalledAt   string   `json:"installed_at,omitempty" yaml:"installed_at,omitempty"`
	LastUsedAt    string   `json:"last_used_at,omitempty" yaml:"last_used_at,omitempty"`
	UserInstalled bool     `json:"user_installed" yaml:"user_installed"`
	// SelectorKind is one of "pin", "channel", "constraint" or "commit".
	SelectorKind    string        `json:"selector_kind" yaml:"selector_kind" enums:"pin,channel,constraint,commit"`
	ResolvedRef     string        `json:"resolved_ref" yaml:"resolved_ref"`
	InstalledCommit string        `json:"installed_commit" yaml:"installed_commit"`
	Available       *AvailableDTO `json:"available,omitempty" yaml:"available,omitempty"`
	// Outdated is true exactly when Available is set.
	Outdated   bool          `json:"outdated" yaml:"outdated"`
	ActiveRun  *RunRecordDTO `json:"active_run,omitempty" yaml:"active_run,omitempty"`
	LastReturn *ReturnDTO    `json:"last_return,omitempty" yaml:"last_return,omitempty"`
	// PendingActivation is a staged binary waiting for a daemon restart, null
	// when nothing is staged.
	PendingActivation *PendingActivationDTO `json:"pending_activation" yaml:"pending_activation"`
	Origin            string                `json:"origin" yaml:"origin"`
	Inference         *InferenceDTO         `json:"inference,omitempty" yaml:"inference,omitempty"`
}

func ArrowDetailDTOFrom(
	a *models.ArrowDetailDTO,
) ArrowDetailDTO {
	installedAt := ""
	if !a.InstalledAt.IsZero() {
		installedAt = a.InstalledAt.Format("2006-01-02T15:04:05Z07:00")
	}
	lastUsedAt := ""
	if !a.LastUsedAt.IsZero() {
		lastUsedAt = a.LastUsedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return ArrowDetailDTO{
		Namespace:       string(a.Namespace),
		Name:            a.Name,
		Description:     a.Description,
		License:         a.License,
		State:           string(a.State),
		Tags:            a.Tags,
		InstalledAt:     installedAt,
		LastUsedAt:      lastUsedAt,
		UserInstalled:   a.UserInstalled,
		SelectorKind:    SelectorKindName(a.SelectorKind),
		ResolvedRef:     a.Resolved.Ref,
		InstalledCommit: a.Resolved.Commit,
		Available:       AvailableDTOFrom(a.Available),
		Outdated:        a.Outdated,
		ActiveRun:       RunRecordDTOFrom(a.ActiveRun),
		LastReturn:      ReturnDTOFrom(a.LastReturn),
		Origin:          a.Origin,
		Inference:       InferenceDTOFrom(a.Generator),

		PendingActivation: PendingActivationDTOFrom(a.PendingActivation),
	}
}
