package ws_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	ws "github.com/rabbytesoftware/quiver/internal/api/v1/ws"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestArrowDTO_FromDomain(t *testing.T) {
	arrow := domain.Arrow{
		Namespace: "github.com/user/repo",
		Manifest: domain.ArrowManifest{
			Name:    "Test",
			Version: "1.0.0",
		},
		Removed: false,
	}
	dto := ws.ArrowDTOFrom(arrow)
	assert.Equal(t, "github.com/user/repo", dto.Namespace)
	assert.Equal(t, "Test", dto.Name)
	assert.Equal(t, "1.0.0", dto.Version)

	data, err := json.Marshal(dto)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"namespace"`)
}

func TestArrowRuntimeDTO_FromDomain(t *testing.T) {
	rt := domainRuntime.ArrowRuntime{
		Namespace: "github.com/user/repo",
		State:     domain.ArrowStateRunning,
	}
	dto := ws.ArrowRuntimeDTOFrom(rt)
	assert.Equal(t, "github.com/user/repo", dto.Namespace)
	assert.Equal(t, "running", dto.State)
	assert.Nil(t, dto.ActiveRun)
	assert.Nil(t, dto.LastReturn)
}

func TestArrowRuntimeDTO_WithActiveRun(t *testing.T) {
	rt := domainRuntime.ArrowRuntime{
		Namespace: "github.com/user/repo",
		State:     domain.ArrowStateRunning,
		ActiveRun: &domainRuntime.RunRecord{
			Method:    "run",
			Variables: map[string]string{"KEY": "val"},
		},
	}
	dto := ws.ArrowRuntimeDTOFrom(rt)
	require.NotNil(t, dto.ActiveRun)
	assert.Equal(t, "run", dto.ActiveRun.Method)
}

func TestQuiverDTO_FromDomain(t *testing.T) {
	q := domain.Quiver{
		Namespace: "github.com/user/repo",
		Manifest:  domain.QuiverManifest{Name: "My Quiver", Tags: []string{"tag1"}},
	}
	dto := ws.QuiverDTOFrom(q)
	assert.Equal(t, "github.com/user/repo", dto.Namespace)
	assert.Equal(t, "My Quiver", dto.Name)
	assert.Equal(t, []string{"tag1"}, dto.Tags)
}
