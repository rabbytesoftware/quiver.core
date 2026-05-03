package mappers_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/app/models/mappers"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

func TestArrowDetailDTOFrom_Nil(t *testing.T) {
	result := mappers.ArrowDetailDTOFrom(nil)
	assert.Nil(t, result)
}

func TestArrowDetailDTOFrom_MapsAllFields(t *testing.T) {
	at := time.Now().UTC()
	outcome := domainRuntime.ExecutionOutcomeSuccess
	view := &models.ArrowDetailView{
		Metadata: domain.Arrow{
			Namespace: "github.com/org/repo@v1.0.0",
			ArrowMeta: domain.ArrowMeta{
				Name:        "Repo",
				Description: "desc",
				Tags:        []string{"t"},
			},
			Variables:           []domain.Variable{{Name: "VAR"}},
			InstalledAt:         at,
			InstalledRef:        "v1.0.0",
			InstalledConstraint: "^1.0.0",
			UserInstalled:       true,
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
	assert.Equal(t, "v1.0.0", result.InstalledRef)
	assert.Equal(t, "^1.0.0", result.InstalledConstraint)
	assert.True(t, result.UserInstalled)
	assert.Equal(t, domain.ArrowStateRunning, result.State)
	assert.Nil(t, result.ActiveRun)
	assert.Equal(t, domain.MethodExecute, result.LastReturn.Method)
}
