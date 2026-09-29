package models

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type AppliedEntry struct {
	Kind     domain.ExposeKind
	Index    int
	Name     string
	Target   string
	Location string
}

type Applied struct {
	Entries []AppliedEntry
	Refused []Refusal
}

func (a *Applied) Record(
	kind domain.ExposeKind,
	index int,
	c Candidate,
	p Placement,
) {
	if p.Refused != "" {
		a.Refuse(kind, index, c.Name, p.Refused)
		return
	}
	a.Entries = append(a.Entries, AppliedEntry{
		Kind:     kind,
		Index:    index,
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
	index int,
	name string,
	reason string,
) {
	a.Refused = append(a.Refused, Refusal{Kind: kind, Index: index, Name: name, Reason: reason})
}
