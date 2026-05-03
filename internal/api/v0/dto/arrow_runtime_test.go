package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

func TestArrowRuntimeDTOFrom(t *testing.T) {
	rt := domainRuntime.ArrowRuntime{
		Ref:   "github.com/user/repo",
		State: domain.ArrowStateRunning,
	}
	d := dto.ArrowRuntimeDTOFrom(rt)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "running", d.State)
	assert.Nil(t, d.ActiveRun)
	assert.Nil(t, d.LastReturn)
}

func TestArrowRuntimeDTOFrom_WithActiveRun(t *testing.T) {
	rt := domainRuntime.ArrowRuntime{
		Ref:   "github.com/user/repo",
		State: domain.ArrowStateRunning,
		Execution: &domainRuntime.Execution{
			Method:    "run",
			Variables: map[string]string{"KEY": "val"},
		},
	}
	d := dto.ArrowRuntimeDTOFrom(rt)
	require.NotNil(t, d.ActiveRun)
	assert.Equal(t, "run", d.ActiveRun.Method)
}
