package dto

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

// DiscoverRequestDTO is the POST body: the query, and nothing else. Discovery
// returns immediately and does not wait for providers.
type DiscoverRequestDTO struct {
	Q string `json:"q"`
}

// DiscoveryJobStartedDTO answers POST /v0/search/discover. It returns before any
// provider has answered, so it carries no counts: those live on the job and are
// read once, after the stream closes.
type DiscoveryJobStartedDTO struct {
	JobID string `json:"job_id"`
	Query string `json:"query"`
	// ExpiresAt is when the job stops being readable. While the pass is still
	// running it is a floor, not a promise: the grace is measured from the
	// moment the pass finishes.
	ExpiresAt time.Time `json:"expires_at"`
}

// DiscoveryJobDTO is everything the stream cannot carry. A stream is one payload
// type, so the counts and the hosts that refused are reported here rather than
// as extra frame shapes.
type DiscoveryJobDTO struct {
	JobID     string                 `json:"job_id"`
	Status    string                 `json:"status"`
	Query     string                 `json:"query"`
	Found     int                    `json:"found"`
	Verified  int                    `json:"verified"`
	Skipped   int                    `json:"skipped"`
	Providers []DiscoveryProviderDTO `json:"providers"`
}

// DiscoveryProviderDTO is what one host contributed. A host that refused is
// reported with the reason it refused, so "GitHub rate-limited you, try in 40s"
// never renders as "no results found".
type DiscoveryProviderDTO struct {
	Host     string `json:"host"`
	OK       bool   `json:"ok"`
	Returned int    `json:"returned"`
	Reason   string `json:"reason,omitempty"`
	// RetryAfter is in seconds and is set only when the host said how long to
	// wait.
	RetryAfter int `json:"retry_after,omitempty"`
}

func DiscoveryJobStartedDTOFrom(
	job usecases.Job,
) DiscoveryJobStartedDTO {
	return DiscoveryJobStartedDTO{
		JobID:     job.ID,
		Query:     job.Query,
		ExpiresAt: job.ExpiresAt,
	}
}

func DiscoveryJobDTOFrom(
	job usecases.Job,
) DiscoveryJobDTO {
	return DiscoveryJobDTO{
		JobID:     job.ID,
		Status:    string(job.Status),
		Query:     job.Query,
		Found:     job.Outcome.Found,
		Verified:  job.Outcome.Verified,
		Skipped:   job.Outcome.Skipped,
		Providers: providerDTOs(job.Outcome.Providers),
	}
}

// providerDTOs keeps an empty provider set rendering as [] rather than null, so
// a client iterates the same way whether or not any host answered.
func providerDTOs(
	outcomes []discovery.ProviderOutcome,
) []DiscoveryProviderDTO {
	dtos := make([]DiscoveryProviderDTO, 0, len(outcomes))
	for _, outcome := range outcomes {
		dtos = append(dtos, DiscoveryProviderDTO{
			Host:       outcome.Host,
			OK:         outcome.OK,
			Returned:   outcome.Returned,
			Reason:     outcome.Reason,
			RetryAfter: int(outcome.RetryAfter.Seconds()),
		})
	}
	return dtos
}
