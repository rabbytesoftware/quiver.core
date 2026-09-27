package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func TestReturnDTOFrom_Nil_ReturnsNil(t *testing.T) {
	assert.Nil(t, dto.ReturnDTOFrom(nil))
}

func TestReturnDTOFrom_CarriesExposed(t *testing.T) {
	got := dto.ReturnDTOFrom(&domainRuntime.Return{
		Method:  domain.MethodInstall,
		Outcome: domainRuntime.ExecutionOutcomeSuccess,
		Exposed: &domainRuntime.ExposeResult{Entries: []domainRuntime.ExposedEntry{{Kind: domain.ExposeKindCLI, Name: "tool"}}},
	})

	require.NotNil(t, got)
	assert.Equal(t, &dto.ExposeResultDTO{
		Entries: []dto.ExposedEntryDTO{{Kind: "cli", Name: "tool"}},
		Refused: []dto.ExposeRefusalDTO{},
	}, got.Exposed)
}

func TestReturnDTO_JSON_OmitsNilExposed(t *testing.T) {
	raw, err := json.Marshal(dto.ReturnDTOFrom(&domainRuntime.Return{Method: domain.MethodInstall}))
	require.NoError(t, err)

	assert.NotContains(t, string(raw), "exposed")
}

func TestReturnDTO_JSON_IncludesExposed(t *testing.T) {
	raw, err := json.Marshal(dto.ReturnDTOFrom(&domainRuntime.Return{
		Method:  domain.MethodInstall,
		Exposed: &domainRuntime.ExposeResult{Entries: []domainRuntime.ExposedEntry{{Kind: domain.ExposeKindCLI, Name: "tool"}}},
	}))
	require.NoError(t, err)

	assert.Contains(t, string(raw), `"exposed":{"entries":[{"kind":"cli","name":"tool","target":"","location":""}],"refused":[]}`)
}
