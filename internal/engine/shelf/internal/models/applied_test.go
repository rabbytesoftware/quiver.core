package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestApplied_RecordAndRefuse(t *testing.T) {
	var a Applied

	a.Record(domain.ExposeKindCLI, Candidate{Name: "rg", Target: "/wd/rg"}, Placement{Location: "/bin/rg"})
	a.Record(domain.ExposeKindDesktop, Candidate{Name: "App", Target: "/wd/App.app"}, Placement{Refused: ReasonUnmanaged})
	a.Refuse(domain.ExposeKindCLI, "x", ReasonNoExecutable)

	assert.Equal(t, []AppliedEntry{{Kind: domain.ExposeKindCLI, Name: "rg", Target: "/wd/rg", Location: "/bin/rg"}}, a.Entries)
	assert.Equal(t, []Refusal{
		{Kind: domain.ExposeKindDesktop, Name: "App", Reason: ReasonUnmanaged},
		{Kind: domain.ExposeKindCLI, Name: "x", Reason: ReasonNoExecutable},
	}, a.Refused)
}

func TestApplied_JSON(t *testing.T) {
	data, err := json.Marshal(Applied{Entries: []AppliedEntry{}, Refused: []Refusal{}})
	require.NoError(t, err)

	assert.JSONEq(t, `{"entries":[],"refused":[]}`, string(data))
}

func TestApplied_Locations(t *testing.T) {
	a := Applied{Entries: []AppliedEntry{{Location: "/a"}, {Location: "/b"}}}

	assert.Equal(t, map[string]bool{"/a": true, "/b": true}, a.Locations())
}

func TestAppliedEntry_JSON(t *testing.T) {
	data, err := json.Marshal(AppliedEntry{Kind: domain.ExposeKindCLI, Name: "rg", Target: "/t", Location: "/l"})
	require.NoError(t, err)

	assert.JSONEq(t, `{"kind":"cli","name":"rg","target":"/t","location":"/l"}`, string(data))
}
