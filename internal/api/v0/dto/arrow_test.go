package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func TestArrowDTOFrom(t *testing.T) {
	a := domain.Arrow{
		Namespace: "github.com/user/repo",
		ArrowMeta: domain.ArrowMeta{Name: "Test", Version: "1.0.0"},
	}
	d := dto.ArrowDTOFrom(a)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "Test", d.Name)
	assert.Equal(t, "1.0.0", d.Version)

	data, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"namespace"`)

	// user_installed must be propagated so arrowWatcher can filter correctly.
	aInstalled := domain.Arrow{
		Namespace:     "github.com/user/repo",
		ArrowMeta:     domain.ArrowMeta{Name: "Test", Version: "1.0.0"},
		UserInstalled: true,
	}
	dInstalled := dto.ArrowDTOFrom(aInstalled)
	assert.True(t, dInstalled.UserInstalled, "UserInstalled must be propagated to ArrowDTO")

	dataInstalled, err := json.Marshal(dInstalled)
	require.NoError(t, err)
	assert.Contains(t, string(dataInstalled), `"user_installed":true`)
}
