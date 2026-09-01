package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func TestArrowDetailDTOFrom(t *testing.T) {
	a := &models.ArrowDetailDTO{
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
	a := &models.ArrowDetailDTO{
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
	a := &models.ArrowDetailDTO{
		Namespace:   domain.Namespace("github.com/user/repo@v1.2.3"),
		InstalledAt: installedTime,
	}
	d := dto.ArrowDetailDTOFrom(a)
	assert.Equal(t, "github.com/user/repo@v1.2.3", d.Namespace)
	assert.Equal(t, "2026-04-21T12:30:45Z", d.InstalledAt)
}

func TestArrowDetailDTOFrom_WithLastUsedAt(t *testing.T) {
	lastUsedTime := time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC)
	a := &models.ArrowDetailDTO{
		Namespace:  domain.Namespace("github.com/user/repo@v1.2.3"),
		LastUsedAt: lastUsedTime,
	}
	d := dto.ArrowDetailDTOFrom(a)
	assert.Equal(t, "2026-08-01T09:30:00Z", d.LastUsedAt)
}

// Which ref an arrow is installed at is the ref its namespace names, so the
// detail response carries no `installed_ref` restating it. What the response
// does have to say is whether that ref is on disk, and `installed_at` is dropped
// entirely rather than sent as a zero time when it is not.
func TestArrowDetailDTO_WireShape_UninstalledOmitsTheStamp(t *testing.T) {
	installed, err := json.Marshal(dto.ArrowDetailDTOFrom(&models.ArrowDetailDTO{
		Namespace:   domain.Namespace("github.com/user/repo@v1.2.3"),
		InstalledAt: time.Date(2026, 4, 21, 12, 30, 45, 0, time.UTC),
	}))
	require.NoError(t, err)

	uninstalled, err := json.Marshal(dto.ArrowDetailDTOFrom(&models.ArrowDetailDTO{
		Namespace: domain.Namespace("github.com/user/repo@v1.2.3"),
	}))
	require.NoError(t, err)

	var withStamp, without map[string]any
	require.NoError(t, json.Unmarshal(installed, &withStamp))
	require.NoError(t, json.Unmarshal(uninstalled, &without))

	assert.NotContains(t, withStamp, "installed_ref")
	assert.NotContains(t, without, "installed_ref")
	assert.Equal(t, "2026-04-21T12:30:45Z", withStamp["installed_at"])
	assert.NotContains(t, without, "installed_at")
	assert.Equal(t, "github.com/user/repo@v1.2.3", without["namespace"])
}

// An arrow that has never been executed must not report a last_used_at at
// all, rather than a zero time.
func TestArrowDetailDTO_WireShape_NeverUsedOmitsTheStamp(t *testing.T) {
	neverUsed, err := json.Marshal(dto.ArrowDetailDTOFrom(&models.ArrowDetailDTO{
		Namespace: domain.Namespace("github.com/user/repo@v1.2.3"),
	}))
	require.NoError(t, err)

	var without map[string]any
	require.NoError(t, json.Unmarshal(neverUsed, &without))

	assert.NotContains(t, without, "last_used_at")
}
