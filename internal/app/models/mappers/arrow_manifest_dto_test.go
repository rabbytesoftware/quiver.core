package mappers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/app/models/mappers"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func TestArrowManifestDTOFrom_Nil(t *testing.T) {
	result := mappers.ArrowManifestDTOFrom(nil)
	assert.Nil(t, result)
}

func TestArrowManifestDTOFrom_MapsAllFields(t *testing.T) {
	arrow := &domain.Arrow{
		Namespace: "github.com/org/repo@v1.0.0",
		ArrowMeta: domain.ArrowMeta{
			Name:        "Repo",
			Description: "desc",
			Version:     "1.0.0",
			Tags:        []string{"tag"},
		},
		Variables: []domain.Variable{{Name: "VAR", Default: "val"}},
	}

	result := mappers.ArrowManifestDTOFrom(arrow)

	require.NotNil(t, result)
	assert.Equal(t, domain.Namespace("github.com/org/repo@v1.0.0"), result.Namespace)
	assert.Equal(t, "Repo", result.Name)
	assert.Equal(t, "desc", result.Description)
	assert.Equal(t, "1.0.0", result.Version)
	assert.Equal(t, []string{"tag"}, result.Tags)
	assert.Equal(t, []domain.Variable{{Name: "VAR", Default: "val"}}, result.Variables)
	assert.Equal(t, arrow, result.Manifest)
}
