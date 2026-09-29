package models

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestApplied_RecordAndRefuse(t *testing.T) {
	var a Applied

	a.Record(domain.ExposeKindCLI, 0, Candidate{Name: "rg", Target: "/wd/rg"}, Placement{Location: "/bin/rg"})
	a.Record(domain.ExposeKindDesktop, 1, Candidate{Name: "App", Target: "/wd/App.app"}, Placement{Refused: ReasonUnmanaged})
	a.Refuse(domain.ExposeKindCLI, 2, "x", ReasonNoExecutable)

	assert.Equal(t, []AppliedEntry{{Kind: domain.ExposeKindCLI, Name: "rg", Target: "/wd/rg", Location: "/bin/rg"}}, a.Entries)
	assert.Equal(t, []Refusal{
		{Kind: domain.ExposeKindDesktop, Index: 1, Name: "App", Reason: ReasonUnmanaged},
		{Kind: domain.ExposeKindCLI, Index: 2, Name: "x", Reason: ReasonNoExecutable},
	}, a.Refused)
}

func TestApplied_Locations(t *testing.T) {
	a := Applied{Entries: []AppliedEntry{{Location: "/a"}, {Location: "/b"}}}

	assert.Equal(t, map[string]bool{"/a": true, "/b": true}, a.Locations())
}
