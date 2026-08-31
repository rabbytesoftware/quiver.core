package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestArrowReadmeDTOFrom_Success(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo")

	result := dto.ArrowReadmeDTOFrom(ns, "# Docs")
	require.NotNil(t, result)
	assert.Equal(t, string(ns), result.Namespace)
	assert.Equal(t, "# Docs", result.Readme)
}
