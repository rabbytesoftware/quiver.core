package shelf

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type Applied struct {
	Entries []AppliedEntry `json:"entries"`
	Refused []Refusal      `json:"refused"`
}

func (a *Applied) record(
	kind domain.ExposeKind,
	c candidate,
	p placement,
) {
	if p.refused != "" {
		a.refuse(kind, c.name, p.refused)
		return
	}
	a.Entries = append(a.Entries, AppliedEntry{
		Kind:     kind,
		Name:     c.name,
		Target:   c.target,
		Location: p.location,
	})
}

func (a *Applied) locations() map[string]bool {
	locations := map[string]bool{}
	for _, e := range a.Entries {
		locations[e.Location] = true
	}
	return locations
}

func (a *Applied) refuse(
	kind domain.ExposeKind,
	name string,
	reason string,
) {
	a.Refused = append(a.Refused, Refusal{Kind: kind, Name: name, Reason: reason})
}
