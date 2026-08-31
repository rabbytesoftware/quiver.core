package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestArrowDependentsDTOFrom_Success(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo")
	dependents := []domain.Namespace{
		"github.com/user/parent-a@v1.0.0",
		"github.com/user/parent-b@v2.0.0",
	}

	result := dto.ArrowDependentsDTOFrom(ns, dependents)
	require.NotNil(t, result)
	assert.Equal(t, string(ns), result.Namespace)
	assert.Equal(t, []string{
		"github.com/user/parent-a@v1.0.0",
		"github.com/user/parent-b@v2.0.0",
	}, result.Dependents)
}

func TestArrowDependentsDTOFrom_Empty(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo")

	result := dto.ArrowDependentsDTOFrom(ns, nil)
	require.NotNil(t, result)
	assert.Equal(t, string(ns), result.Namespace)
	assert.Empty(t, result.Dependents)
}
