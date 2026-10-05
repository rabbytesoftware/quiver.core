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

func TestRunRecordDTO_JSON_EmptyStepsSerializeAsArray(t *testing.T) {
	raw, err := json.Marshal(dto.RunRecordDTOFrom(&domainRuntime.Execution{Method: domain.MethodUninstall}))
	require.NoError(t, err)

	assert.Contains(t, string(raw), `"steps":[]`)
}

func TestRunRecordDTOFrom_Surface(t *testing.T) {
	got := dto.RunRecordDTOFrom(&domainRuntime.Execution{
		Method: "execute",
		Surface: &domainRuntime.Surface{
			Mode: domainRuntime.SurfaceModeStatic, Path: "/", Dir: "/secret/dir", Ready: true,
		},
	})
	require.Equal(t, &dto.SurfaceDTO{Mode: "static", Path: "/", Ready: true}, got.Surface)

	none := dto.RunRecordDTOFrom(&domainRuntime.Execution{Method: "install"})
	require.Nil(t, none.Surface)
}
