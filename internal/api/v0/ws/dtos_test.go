package ws_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArrowDTOFrom_MapsAllFields(t *testing.T) {
	a := domain.Arrow{
		Namespace: "github.com/user/repo",
		Manifest: domain.ArrowManifest{
			Name:        "MyArrow",
			Version:     "2.0.0",
			Description: "desc",
			Tags:        []string{"a", "b"},
		},
	}

	d := dto.ArrowDTOFrom(a)

	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "MyArrow", d.Name)
	assert.Equal(t, "2.0.0", d.Version)
	assert.Equal(t, "desc", d.Description)
	assert.Equal(t, []string{"a", "b"}, d.Tags)
}

func TestArrowRuntimeDTOFrom_NilRunAndReturn(t *testing.T) {
	rt := domainRuntime.ArrowRuntime{
		Namespace:  "github.com/user/repo",
		State:      domain.ArrowStateReady,
		ActiveRun:  nil,
		LastReturn: nil,
	}

	d := dto.ArrowRuntimeDTOFrom(rt)

	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Nil(t, d.ActiveRun)
	assert.Nil(t, d.LastReturn)
}

func TestArrowRuntimeDTOFrom_WithActiveRun(t *testing.T) {
	rt := domainRuntime.ArrowRuntime{
		Namespace: "github.com/user/repo",
		State:     domain.ArrowStateRunning,
		ActiveRun: &domainRuntime.RunRecord{
			Method:    "execute",
			Variables: map[string]string{"KEY": "val"},
		},
	}

	d := dto.ArrowRuntimeDTOFrom(rt)

	require.NotNil(t, d.ActiveRun)
	assert.Equal(t, "execute", d.ActiveRun.Method)
	assert.Equal(t, map[string]string{"KEY": "val"}, d.ActiveRun.Variables)
	assert.Nil(t, d.LastReturn)
}

func TestArrowRuntimeDTOFrom_WithLastReturn(t *testing.T) {
	rt := domainRuntime.ArrowRuntime{
		Namespace: "github.com/user/repo",
		State:     domain.ArrowStateReady,
		LastReturn: &domainRuntime.Return{
			Method:    "execute",
			Outcome:   domainRuntime.ExecutionOutcomeSuccess,
			Variables: map[string]string{"OUT": "result"},
		},
	}

	d := dto.ArrowRuntimeDTOFrom(rt)

	assert.Nil(t, d.ActiveRun)
	require.NotNil(t, d.LastReturn)
	assert.Equal(t, "execute", d.LastReturn.Method)
	assert.Equal(t, "success", d.LastReturn.Outcome)
	assert.Equal(t, map[string]string{"OUT": "result"}, d.LastReturn.Variables)
}

func TestArrowRuntimeDTOFrom_WithBothFields(t *testing.T) {
	rt := domainRuntime.ArrowRuntime{
		Namespace: "github.com/user/repo",
		State:     domain.ArrowStateRunning,
		ActiveRun: &domainRuntime.RunRecord{
			Method: "run",
		},
		LastReturn: &domainRuntime.Return{
			Method:  "prev",
			Outcome: domainRuntime.ExecutionOutcomeFailed,
		},
	}

	d := dto.ArrowRuntimeDTOFrom(rt)

	require.NotNil(t, d.ActiveRun)
	assert.Equal(t, "run", d.ActiveRun.Method)
	require.NotNil(t, d.LastReturn)
	assert.Equal(t, "prev", d.LastReturn.Method)
	assert.Equal(t, "failed", d.LastReturn.Outcome)
}

func TestQuiverDTOFrom_MapsAllFields(t *testing.T) {
	q := domain.Quiver{
		Namespace: "github.com/org/quiver",
		Manifest: domain.QuiverManifest{
			Name:        "MyQuiver",
			Description: "quiver desc",
			Tags:        []string{"x", "y"},
		},
	}

	d := dto.QuiverDTOFrom(q)

	assert.Equal(t, "github.com/org/quiver", d.Namespace)
	assert.Equal(t, "MyQuiver", d.Name)
	assert.Equal(t, "quiver desc", d.Description)
	assert.Equal(t, []string{"x", "y"}, d.Tags)
}
