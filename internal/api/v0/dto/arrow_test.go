package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArrowDTOFrom(t *testing.T) {
	a := domain.Arrow{
		Namespace: "github.com/user/repo",
		Versions: map[string]domain.ArrowManifest{
			"1.0.0": {ArrowMeta: domain.ArrowMeta{Name: "Test", Version: "1.0.0"}},
		},
	}
	d := dto.ArrowDTOFrom(a)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "Test", d.Name)
	assert.Equal(t, "1.0.0", d.Version)

	data, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"namespace"`)
}
