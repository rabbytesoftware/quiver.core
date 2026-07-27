package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

func TestDiscoveryJobStartedDTOFrom(t *testing.T) {
	expires := time.Date(2026, 7, 26, 12, 0, 30, 0, time.UTC)

	d := dto.DiscoveryJobStartedDTOFrom(usecases.Job{
		ID:        "job-1",
		Query:     "chrom",
		Status:    usecases.JobRunning,
		ExpiresAt: expires,
	})

	assert.Equal(t, "job-1", d.JobID)
	assert.Equal(t, "chrom", d.Query)
	assert.True(t, expires.Equal(d.ExpiresAt))
}

func TestDiscoveryJobDTOFrom_CompletedCarriesCountsAndProviders(t *testing.T) {
	d := dto.DiscoveryJobDTOFrom(usecases.Job{
		ID:     "job-1",
		Query:  "chrom",
		Status: usecases.JobCompleted,
		Outcome: discovery.Outcome{
			Found:    25,
			Verified: 19,
			Skipped:  6,
			Providers: []discovery.ProviderOutcome{
				{Host: "github.com", OK: true, Returned: 25},
				{
					Host:       "gitlab.com",
					Reason:     discovery.ReasonRateLimited,
					RetryAfter: 40 * time.Second,
				},
			},
		},
	})

	assert.Equal(t, "job-1", d.JobID)
	assert.Equal(t, "completed", d.Status)
	assert.Equal(t, "chrom", d.Query)
	assert.Equal(t, 25, d.Found)
	assert.Equal(t, 19, d.Verified)
	assert.Equal(t, 6, d.Skipped)

	require.Len(t, d.Providers, 2)
	assert.Equal(t, dto.DiscoveryProviderDTO{Host: "github.com", OK: true, Returned: 25}, d.Providers[0])
	assert.Equal(t, dto.DiscoveryProviderDTO{
		Host:       "gitlab.com",
		Reason:     "rate_limited",
		RetryAfter: 40,
	}, d.Providers[1])
}

// TestDiscoveryJobDTOFrom_RunningHasNoProvidersButRendersAList keeps a client
// iterating the same way before and after the pass finishes.
func TestDiscoveryJobDTOFrom_RunningHasNoProvidersButRendersAList(t *testing.T) {
	d := dto.DiscoveryJobDTOFrom(usecases.Job{
		ID:     "job-1",
		Query:  "chrom",
		Status: usecases.JobRunning,
	})

	assert.Equal(t, "running", d.Status)
	require.NotNil(t, d.Providers)
	assert.Empty(t, d.Providers)

	encoded, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"providers":[]`)
	assert.NotContains(t, string(encoded), `"reason"`)
	assert.NotContains(t, string(encoded), `"retry_after"`)
}

// TestDiscoveryJobDTOFrom_SubSecondRetryRoundsDown documents the wire unit: a
// retry hint is whole seconds, and a hint below one second reports zero rather
// than a fraction the client cannot render.
func TestDiscoveryJobDTOFrom_SubSecondRetryRoundsDown(t *testing.T) {
	d := dto.DiscoveryJobDTOFrom(usecases.Job{
		Outcome: discovery.Outcome{
			Providers: []discovery.ProviderOutcome{
				{Host: "gitlab.com", Reason: discovery.ReasonError, RetryAfter: 900 * time.Millisecond},
			},
		},
	})

	require.Len(t, d.Providers, 1)
	assert.Zero(t, d.Providers[0].RetryAfter)
	assert.Equal(t, "error", d.Providers[0].Reason)
}
