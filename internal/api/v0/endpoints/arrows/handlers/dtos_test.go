package arrows_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	arrows "github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/arrows/handlers"
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestExecuteMethodRequestDTO_MarshalUnmarshal(t *testing.T) {
	original := arrows.ExecuteMethodRequestDTO{
		Variables: map[string]string{"KEY": "value", "PORT": "8080"},
	}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded arrows.ExecuteMethodRequestDTO
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original.Variables, decoded.Variables)
}

func TestExecuteMethodRequestDTO_UnmarshalEmptyJSON(t *testing.T) {
	var dto arrows.ExecuteMethodRequestDTO
	require.NoError(t, json.Unmarshal([]byte(`{}`), &dto))
	assert.Nil(t, dto.Variables)
}

func TestToArrowListItemDTO(t *testing.T) {
	app := arrow.ArrowListDTO{
		Namespace:   domain.Namespace("github.com/user/repo"),
		Name:        "My Arrow",
		Version:     "1.0.0",
		Description: "desc",
		State:       domain.ArrowStateReady,
		Tags:        []string{"tag1"},
		Removed:     false,
	}
	dto := arrows.ToArrowListItemDTO(app)
	assert.Equal(t, "github.com/user/repo", dto.Namespace)
	assert.Equal(t, "My Arrow", dto.Name)
	assert.Equal(t, "1.0.0", dto.Version)
	assert.Equal(t, "ready", dto.State)
	assert.Equal(t, []string{"tag1"}, dto.Tags)
	assert.False(t, dto.Removed)
}

func TestToArrowDetailDTO(t *testing.T) {
	app := &arrow.ArrowDetailDTO{
		Namespace: domain.Namespace("github.com/user/repo"),
		Manifest: domain.ArrowManifest{
			Name:        "My Arrow",
			Version:     "1.0.0",
			Description: "desc",
			Tags:        []string{"tag1"},
		},
		State:                domain.ArrowStateReady,
		Removed:              false,
		IndirectDependencies: []domain.Namespace{"github.com/dep/one"},
	}
	dto := arrows.ToArrowDetailDTO(app)
	assert.Equal(t, "github.com/user/repo", dto.Namespace)
	assert.Equal(t, "My Arrow", dto.Name)
	assert.Equal(t, "ready", dto.State)
	assert.Equal(t, []string{"github.com/dep/one"}, dto.IndirectDependencies)
	assert.Nil(t, dto.ActiveRun)
}

func TestToArrowDetailDTO_WithActiveRunAndReturn(t *testing.T) {
	activeRun := &domainRuntime.RunRecord{
		Method:    "run",
		Variables: map[string]string{"KEY": "val"},
	}
	lastReturn := &domainRuntime.Return{
		Method:  "run",
		Outcome: domainRuntime.ExecutionOutcomeSuccess,
	}
	app := &arrow.ArrowDetailDTO{
		Namespace:  domain.Namespace("github.com/user/repo"),
		ActiveRun:  activeRun,
		LastReturn: lastReturn,
	}
	dto := arrows.ToArrowDetailDTO(app)
	require.NotNil(t, dto.ActiveRun)
	assert.Equal(t, "run", dto.ActiveRun.Method)
	require.NotNil(t, dto.LastReturn)
	assert.Equal(t, "success", dto.LastReturn.Outcome)
}
