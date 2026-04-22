package dto_test

import (
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArrowDetailDTOFrom(t *testing.T) {
	a := &arrow.ArrowDetailDTO{
		Namespace: domain.Namespace("github.com/user/repo"),
		Name:      "My Arrow",
		Tags:      []string{"tag1"},
		State:     domain.ArrowStateReady,
	}
	d := dto.ArrowDetailDTOFrom(a)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "My Arrow", d.Name)
	assert.Equal(t, "ready", d.State)
	assert.Nil(t, d.ActiveRun)
}

func TestArrowDetailDTOFrom_WithActiveRunAndReturn(t *testing.T) {
	a := &arrow.ArrowDetailDTO{
		Namespace: domain.Namespace("github.com/user/repo"),
		ActiveRun: &domainRuntime.Execution{
			Method:    "run",
			Variables: map[string]string{"KEY": "val"},
		},
		LastReturn: &domainRuntime.Return{
			Method:  "run",
			Outcome: domainRuntime.ExecutionOutcomeSuccess,
		},
	}
	d := dto.ArrowDetailDTOFrom(a)
	require.NotNil(t, d.ActiveRun)
	assert.Equal(t, "run", d.ActiveRun.Method)
	require.NotNil(t, d.LastReturn)
	assert.Equal(t, "success", d.LastReturn.Outcome)
}

func TestArrowDetailDTOFrom_WithInstalledAt(t *testing.T) {
	installedTime := time.Date(2026, 4, 21, 12, 30, 45, 0, time.UTC)
	a := &arrow.ArrowDetailDTO{
		Namespace:    domain.Namespace("github.com/user/repo"),
		InstalledRef: "v1.2.3",
		InstalledAt:  installedTime,
	}
	d := dto.ArrowDetailDTOFrom(a)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
	assert.Equal(t, "v1.2.3", d.InstalledRef)
	assert.Equal(t, "2026-04-21T12:30:45Z", d.InstalledAt)
}
