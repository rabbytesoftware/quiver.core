package shelf

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestApplied_RecordAndRefuse(t *testing.T) {
	var a Applied

	a.record(domain.ExposeKindCLI, candidate{name: "rg", target: "/wd/rg"}, placement{location: "/bin/rg"})
	a.record(domain.ExposeKindDesktop, candidate{name: "App", target: "/wd/App.app"}, placement{refused: reasonUnmanaged})
	a.refuse(domain.ExposeKindCLI, "x", reasonNoExecutable)

	assert.Equal(t, []AppliedEntry{{Kind: domain.ExposeKindCLI, Name: "rg", Target: "/wd/rg", Location: "/bin/rg"}}, a.Entries)
	assert.Equal(t, []Refusal{
		{Kind: domain.ExposeKindDesktop, Name: "App", Reason: reasonUnmanaged},
		{Kind: domain.ExposeKindCLI, Name: "x", Reason: reasonNoExecutable},
	}, a.Refused)
}

func TestApplied_JSON(t *testing.T) {
	data, err := json.Marshal(Applied{Entries: []AppliedEntry{}, Refused: []Refusal{}})
	require.NoError(t, err)

	assert.JSONEq(t, `{"entries":[],"refused":[]}`, string(data))
}
