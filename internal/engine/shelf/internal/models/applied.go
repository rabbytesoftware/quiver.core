package models

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type AppliedEntry struct {
	Kind     domain.ExposeKind `json:"kind"`
	Name     string            `json:"name"`
	Target   string            `json:"target"`
	Location string            `json:"location"`
}

type Applied struct {
	Entries []AppliedEntry `json:"entries"`
	Refused []Refusal      `json:"refused"`
}

func (a *Applied) Record(
	kind domain.ExposeKind,
	c Candidate,
	p Placement,
) {
	if p.Refused != "" {
		a.Refuse(kind, c.Name, p.Refused)
		return
	}
	a.Entries = append(a.Entries, AppliedEntry{
		Kind:     kind,
		Name:     c.Name,
		Target:   c.Target,
		Location: p.Location,
	})
}

func (a *Applied) Locations() map[string]bool {
	locations := map[string]bool{}
	for _, e := range a.Entries {
		locations[e.Location] = true
	}
	return locations
}

func (a *Applied) Refuse(
	kind domain.ExposeKind,
	name string,
	reason string,
) {
	a.Refused = append(a.Refused, Refusal{Kind: kind, Name: name, Reason: reason})
}
