package mappers_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/models/mappers"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func TestArrowDetailDTOFrom_Nil(t *testing.T) {
	result := mappers.ArrowDetailDTOFrom(nil)
	assert.Nil(t, result)
}

func TestArrowDetailDTOFrom_MapsAllFields(t *testing.T) {
	at := time.Now().UTC()
	lastUsed := at.Add(time.Hour)
	outcome := domainRuntime.ExecutionOutcomeSuccess
	view := &models.ArrowDetailView{
		Metadata: domain.Arrow{
			Namespace: "github.com/org/repo@v1.0.0",
			ArrowMeta: domain.ArrowMeta{
				Name:        "Repo",
				Description: "desc",
				Tags:        []string{"t"},
			},
			Variables:     []domain.Variable{{Name: "VAR"}},
			InstalledAt:   at,
			LastUsedAt:    lastUsed,
			UserInstalled: true,
			Available:     &domain.Available{Ref: "v2.0.0", Commit: "c2"},
		},
		State: domain.ArrowStateRunning,
		LastReturn: &domainRuntime.Return{
			Method:  domain.MethodExecute,
			Outcome: outcome,
		},
	}

	result := mappers.ArrowDetailDTOFrom(view)

	require.NotNil(t, result)
	assert.Equal(t, domain.Namespace("github.com/org/repo@v1.0.0"), result.Namespace)
	assert.Equal(t, "Repo", result.Name)
	assert.Equal(t, "desc", result.Description)
	assert.Equal(t, []string{"t"}, result.Tags)
	assert.Equal(t, []domain.Variable{{Name: "VAR"}}, result.Variables)
	assert.Equal(t, at, result.InstalledAt)
	assert.Equal(t, lastUsed, result.LastUsedAt)
	assert.True(t, result.UserInstalled)
	assert.True(t, result.Outdated)
	assert.Equal(t, domain.ArrowStateRunning, result.State)
	assert.Nil(t, result.ActiveRun)
	assert.Equal(t, domain.MethodExecute, result.LastReturn.Method)
}

func TestArrowDetailDTOFrom_NeverUsed_LastUsedAtIsZero(t *testing.T) {
	view := &models.ArrowDetailView{
		Metadata: domain.Arrow{Namespace: "github.com/org/repo@v1.0.0"},
	}

	result := mappers.ArrowDetailDTOFrom(view)

	require.NotNil(t, result)
	assert.True(t, result.LastUsedAt.IsZero())
	assert.False(t, result.Outdated)
}

func TestArrowDetailDTOFrom_CarriesVersioning(t *testing.T) {
	available := &domain.Available{Ref: "v1.1.0", Commit: "c2"}
	view := &models.ArrowDetailView{
		Metadata: domain.Arrow{
			Namespace:    "github.com/org/repo@stable",
			SelectorKind: domain.SelectorChannel,
			Resolved:     domain.Resolved{Ref: "v1.0.0", Commit: "c1", Fingerprint: "f"},
			Available:    available,
		},
	}

	result := mappers.ArrowDetailDTOFrom(view)

	require.NotNil(t, result)
	assert.Equal(t, domain.SelectorChannel, result.SelectorKind)
	assert.Equal(t, domain.Resolved{Ref: "v1.0.0", Commit: "c1", Fingerprint: "f"}, result.Resolved)
	assert.Equal(t, available, result.Available)
	assert.True(t, result.Outdated)
}

func TestArrowDetailDTOFrom_LegacyZeroRow_HasNoVersioning(t *testing.T) {
	result := mappers.ArrowDetailDTOFrom(&models.ArrowDetailView{
		Metadata: domain.Arrow{Namespace: "github.com/org/repo@v1.0.0"},
	})

	require.NotNil(t, result)
	assert.Equal(t, domain.SelectorPin, result.SelectorKind)
	assert.Empty(t, result.Resolved.Ref)
	assert.Nil(t, result.Available)
}
