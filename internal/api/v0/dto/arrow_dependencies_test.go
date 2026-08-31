package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestArrowDependenciesDTOFrom_Success(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	plan := models.Plan{
		{Namespace: "github.com/user/tool-dep@v1.0.0", Type: domain.ToolDep},
		{Namespace: "github.com/user/service-dep@v2.0.0", Type: domain.ServiceDep},
	}

	result := dto.ArrowDependenciesDTOFrom(ns, plan)
	require.NotNil(t, result)
	assert.Equal(t, string(ns), result.Namespace)
	require.Len(t, result.Dependencies, 2)
	assert.Equal(t, "github.com/user/tool-dep@v1.0.0", result.Dependencies[0].Namespace)
	assert.Equal(t, "tool", result.Dependencies[0].Type)
	assert.Equal(t, "github.com/user/service-dep@v2.0.0", result.Dependencies[1].Namespace)
	assert.Equal(t, "service", result.Dependencies[1].Type)
}

func TestArrowDependenciesDTOFrom_Empty(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")

	result := dto.ArrowDependenciesDTOFrom(ns, nil)
	require.NotNil(t, result)
	assert.Equal(t, string(ns), result.Namespace)
	assert.Empty(t, result.Dependencies)
}
