package mappers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/models/mappers"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
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
			Tags:        []string{"tag"},
		},
		Variables: []domain.Variable{{Name: "VAR", Default: "val"}},
	}

	result := mappers.ArrowManifestDTOFrom(arrow)

	require.NotNil(t, result)
	assert.Equal(t, domain.Namespace("github.com/org/repo@v1.0.0"), result.Namespace)
	assert.Equal(t, "Repo", result.Name)
	assert.Equal(t, "desc", result.Description)
	assert.Equal(t, "v1.0.0", result.Namespace.Ref(), "the ref the manifest was resolved at is its version")
	assert.Equal(t, []string{"tag"}, result.Tags)
	assert.Equal(t, []domain.Variable{{Name: "VAR", Default: "val"}}, result.Variables)
	assert.Equal(t, arrow, result.Manifest)
}

// Manifest embeds *domain.Arrow verbatim, and that is exactly where the
// resolver stamps RefIsBranch/RefCommitSHA — without stripping them here, a
// branch-tracked arrow's manifest response would leak straight from storage
// the same internal fact the design deliberately keeps off every DTO.
func TestArrowManifestDTOFrom_StripsRefMutabilityFields(t *testing.T) {
	arrow := &domain.Arrow{
		Namespace:    "github.com/org/repo@develop",
		RefIsBranch:  true,
		RefCommitSHA: "abc123",
	}

	result := mappers.ArrowManifestDTOFrom(arrow)

	require.NotNil(t, result)
	require.NotNil(t, result.Manifest)
	assert.False(t, result.Manifest.RefIsBranch)
	assert.Empty(t, result.Manifest.RefCommitSHA)
	// The source arrow itself must be untouched — only the DTO's copy strips.
	assert.True(t, arrow.RefIsBranch)
	assert.Equal(t, "abc123", arrow.RefCommitSHA)
}
