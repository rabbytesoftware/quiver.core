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

func stagedAt() time.Time {
	return time.Date(2026, 10, 3, 12, 30, 0, 0, time.FixedZone("x", -3*3600))
}

func TestPendingActivationDTOFrom(t *testing.T) {
	assert.Nil(t, dto.PendingActivationDTOFrom(nil))

	got := dto.PendingActivationDTOFrom(&domainRuntime.PendingActivation{
		Version:  "nightly-2",
		StagedAt: stagedAt(),
		Path:     "/never/exposed",
		Digest:   "never exposed",
	})

	require.NotNil(t, got)
	assert.Equal(t, "nightly-2", got.Version)
	assert.Equal(t, "2026-10-03T15:30:00Z", got.StagedAt, "RFC 3339 in UTC")
}

func TestArrowRuntimeDTOFrom_CarriesThePendingActivation(t *testing.T) {
	rt := domainRuntime.ArrowRuntime{
		Ref:               "github.com/user/repo",
		State:             domain.ArrowStateReady,
		PendingActivation: &domainRuntime.PendingActivation{Version: "v2", StagedAt: stagedAt(), Path: "/p"},
	}

	raw, err := json.Marshal(dto.ArrowRuntimeDTOFrom(rt))
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(raw, &wire))
	assert.Equal(t, map[string]any{"version": "v2", "staged_at": "2026-10-03T15:30:00Z"}, wire["pending_activation"])
}

func TestArrowRuntimeDTOFrom_NothingStagedIsNull(t *testing.T) {
	raw, err := json.Marshal(dto.ArrowRuntimeDTOFrom(domainRuntime.ArrowRuntime{Ref: "github.com/user/repo", State: domain.ArrowStateReady}))
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(raw, &wire))
	value, present := wire["pending_activation"]
	assert.True(t, present, "the key is always there")
	assert.Nil(t, value)
}

func TestArrowDetailDTOFrom_CarriesThePendingActivation(t *testing.T) {
	a := &models.ArrowDetailDTO{
		Namespace:         "github.com/user/repo",
		PendingActivation: &domainRuntime.PendingActivation{Version: "v2", StagedAt: stagedAt()},
	}

	raw, err := json.Marshal(dto.ArrowDetailDTOFrom(a))
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(raw, &wire))
	assert.Equal(t, map[string]any{"version": "v2", "staged_at": "2026-10-03T15:30:00Z"}, wire["pending_activation"])

	raw, err = json.Marshal(dto.ArrowDetailDTOFrom(&models.ArrowDetailDTO{Namespace: "github.com/user/repo"}))
	require.NoError(t, err)
	wire = nil
	require.NoError(t, json.Unmarshal(raw, &wire))
	value, present := wire["pending_activation"]
	assert.True(t, present)
	assert.Nil(t, value)
}
