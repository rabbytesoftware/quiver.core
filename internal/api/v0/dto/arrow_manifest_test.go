package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func TestArrowManifestDTOFrom_Nil_ReturnsNil(t *testing.T) {
	result := dto.ArrowManifestDTOFrom(nil)
	assert.Nil(t, result)
}

func TestArrowManifestDTOFrom_Success(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	arrow := &domain.Arrow{Namespace: ns}
	input := &models.ArrowManifestDTO{
		Namespace:   ns,
		Name:        "Test Arrow",
		Description: "A test arrow",
		Version:     "v1.0.0",
		Tags:        []string{"tag1", "tag2"},
		Variables:   []domain.Variable{{Name: "VAR", Default: "val"}},
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinARM64: {},
		},
		Manifest: arrow,
	}

	result := dto.ArrowManifestDTOFrom(input)
	require.NotNil(t, result)
	assert.Equal(t, string(ns), result.Namespace)
	assert.Equal(t, "Test Arrow", result.Name)
	assert.Equal(t, "A test arrow", result.Description)
	assert.Equal(t, "v1.0.0", result.Version)
	assert.Equal(t, []string{"tag1", "tag2"}, result.Tags)
	assert.Len(t, result.Variables, 1)
	assert.Len(t, result.Targets, 1)
	assert.Equal(t, arrow, result.Manifest)
}
