package dto

import domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"

type ExposeResultDTO struct {
	Entries []ExposedEntryDTO  `json:"entries" yaml:"entries"`
	Refused []ExposeRefusalDTO `json:"refused" yaml:"refused"`
}

type ExposedEntryDTO struct {
	Kind     string `json:"kind" yaml:"kind"`
	Name     string `json:"name" yaml:"name"`
	Target   string `json:"target" yaml:"target"`
	Location string `json:"location" yaml:"location"`
}

type ExposeRefusalDTO struct {
	Kind   string `json:"kind" yaml:"kind"`
	Name   string `json:"name" yaml:"name"`
	Reason string `json:"reason" yaml:"reason"`
}

func ExposeResultDTOFrom(
	r *domainRuntime.ExposeResult,
) *ExposeResultDTO {
	if r == nil {
		return nil
	}
	out := &ExposeResultDTO{
		Entries: make([]ExposedEntryDTO, 0, len(r.Entries)),
		Refused: make([]ExposeRefusalDTO, 0, len(r.Refused)),
	}
	for _, e := range r.Entries {
		out.Entries = append(out.Entries, ExposedEntryDTO{
			Kind:     string(e.Kind),
			Name:     e.Name,
			Target:   e.Target,
			Location: e.Location,
		})
	}
	for _, rf := range r.Refused {
		out.Refused = append(out.Refused, ExposeRefusalDTO{
			Kind:   string(rf.Kind),
			Name:   rf.Name,
			Reason: rf.Reason,
		})
	}
	return out
}
