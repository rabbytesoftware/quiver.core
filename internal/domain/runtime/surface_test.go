package runtime_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func TestSurface_JSONRoundTripOnExecution(t *testing.T) {
	exec := domainRuntime.Execution{
		ID:     "e1",
		Method: "execute",
		Surface: &domainRuntime.Surface{
			Mode:  domainRuntime.SurfaceModeListen,
			Path:  "/",
			Ready: true,
		},
	}

	raw, err := json.Marshal(exec)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"surface":{"mode":"listen","path":"/","ready":true}`)

	var back domainRuntime.Execution
	require.NoError(t, json.Unmarshal(raw, &back))
	require.Equal(t, exec.Surface, back.Surface)
}

func TestExecution_NoSurfaceOmitsField(t *testing.T) {
	raw, err := json.Marshal(domainRuntime.Execution{ID: "e1", Method: "install"})
	require.NoError(t, err)
	require.NotContains(t, string(raw), "surface")
}
