package dto_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func TestSurfaceDTOFrom_OmitsTheServedDirectory(t *testing.T) {
	got := dto.SurfaceDTOFrom(&domainRuntime.Surface{
		Mode:  domainRuntime.SurfaceModeStatic,
		Path:  "/",
		Dir:   "/secret/dir",
		Ready: true,
	})

	require.Equal(t, &dto.SurfaceDTO{Mode: "static", Path: "/", Ready: true}, got)
}

func TestSurfaceDTOFrom_NilIsNil(t *testing.T) {
	require.Nil(t, dto.SurfaceDTOFrom(nil))
}

func TestRunRecordDTOFrom_CarriesTheSurface(t *testing.T) {
	got := dto.RunRecordDTOFrom(&domainRuntime.Execution{
		Method:  "execute",
		Surface: &domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen, Path: "/"},
	})

	require.Equal(t, &dto.SurfaceDTO{Mode: "listen", Path: "/"}, got.Surface)
}
