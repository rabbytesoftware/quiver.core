package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// UpdateResultDTO reports what PATCH /arrow/{ns} did. A row nothing is
// installed from advances at once and reports its dependency changes; an
// installed row stays put and reports only Available, which the runtime update
// then moves it to.
type UpdateResultDTO struct {
	AddedDeps           []string            `json:"added_deps" yaml:"added_deps"`
	RemovedFromManifest []string            `json:"removed_from_manifest" yaml:"removed_from_manifest"`
	SafeToUninstall     []string            `json:"safe_to_uninstall" yaml:"safe_to_uninstall"`
	ConstrainedDeps     []ConstrainedDepDTO `json:"constrained_deps" yaml:"constrained_deps"`
	Available           *AvailableDTO       `json:"available,omitempty" yaml:"available,omitempty"`
}

// ConstrainedDepDTO is a dependency whose version constraint the update changed.
type ConstrainedDepDTO struct {
	Namespace     string `json:"namespace" yaml:"namespace"`
	OldConstraint string `json:"old_constraint" yaml:"old_constraint"`
	NewConstraint string `json:"new_constraint" yaml:"new_constraint"`
}

func UpdateResultDTOFrom(r models.UpdateResult) UpdateResultDTO {
	constrained := make([]ConstrainedDepDTO, 0, len(r.ConstrainedDeps))
	for _, c := range r.ConstrainedDeps {
		constrained = append(constrained, ConstrainedDepDTO{
			Namespace:     string(c.Namespace),
			OldConstraint: c.OldConstraint,
			NewConstraint: c.NewConstraint,
		})
	}
	return UpdateResultDTO{
		AddedDeps:           namespaceStrings(r.AddedDeps),
		RemovedFromManifest: namespaceStrings(r.RemovedFromManifest),
		SafeToUninstall:     namespaceStrings(r.SafeToUninstall),
		ConstrainedDeps:     constrained,
		Available:           AvailableDTOFrom(r.Available),
	}
}

func namespaceStrings(nss []domain.Namespace) []string {
	out := make([]string, 0, len(nss))
	for _, ns := range nss {
		out = append(out, string(ns))
	}
	return out
}
