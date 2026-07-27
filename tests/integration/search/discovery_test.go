//go:build integration

package search_test

import (
	"context"
	"fmt"
	"net/http"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

// providerFor finds one host's outcome in a job summary.
func providerFor(
	summary apidto.DiscoveryJobDTO,
	host string,
) (apidto.DiscoveryProviderDTO, bool) {
	for _, p := range summary.Providers {
		if p.Host == host {
			return p, true
		}
	}
	return apidto.DiscoveryProviderDTO{}, false
}

// --------------------------------------------------------------- Lane B -----

func (s *SearchSuite) TestDiscover_Start_ReturnsAJobAndCompletes() {
	prov := newStubProvider(fixtureHost).answer(
		"widget",
		candidateFor("search-widget-beta", 12),
		candidateFor("search-widget-alpha", 34),
	)
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)
	s.Require().NotEmpty(job.JobID)
	s.Equal("widget", job.Query)
	s.False(job.ExpiresAt.IsZero(), "a started job says when it stops being readable")

	summary := s.waitForCompleted(tc, job.JobID)
	s.Equal(job.JobID, summary.JobID)
	s.Equal("widget", summary.Query)
	s.Equal(2, summary.Found)
	s.Equal(2, summary.Verified)
	s.Zero(summary.Skipped)

	s.Require().Len(summary.Providers, 1)
	s.Equal(fixtureHost, summary.Providers[0].Host)
	s.True(summary.Providers[0].OK)
	s.Equal(2, summary.Providers[0].Returned)
	s.Empty(summary.Providers[0].Reason)

	s.Equal(metadata.GetDiscovery().Topics, prov.topicsAsked(),
		"a candidate is asked for by the discovery markers, never by a hardcoded topic")
}

func (s *SearchSuite) TestDiscover_Start_JobsGetDistinctIDs() {
	prov := newStubProvider(fixtureHost).answer("widget", candidateFor("search-widget-beta", 1))
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	first, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)
	second, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)

	s.NotEqual(first.JobID, second.JobID)
	s.waitForCompleted(tc, first.JobID)
	s.waitForCompleted(tc, second.JobID)
}

func (s *SearchSuite) TestDiscover_Job_RunningBeforeTheProviderAnswers() {
	prov := newStubProvider(fixtureHost)
	gate := prov.gated("widget", candidateFor("search-widget-beta", 1))
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)

	running, status := tc.DiscoveryJob(job.JobID)
	s.Equal(http.StatusOK, status)
	s.Equal(string(usecases.JobRunning), running.Status,
		"the pass is reported as running until the host answers")
	s.Zero(running.Found)

	close(gate)
	summary := s.waitForCompleted(tc, job.JobID)
	s.Equal(1, summary.Verified)
}

func (s *SearchSuite) TestDiscover_Discovery_FeedsLaneA() {
	prov := newStubProvider(fixtureHost).answer("lumen", candidateFor("search-lumen", 7))
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	before, status := tc.Search("lumen", kit.SearchParams{})
	s.Require().Equal(http.StatusOK, status)
	s.Require().Empty(before)

	s.runDiscovery(tc, "lumen", 1)

	after, status := tc.Search("lumen", kit.SearchParams{})
	s.Equal(http.StatusOK, status)
	s.Require().Len(after, 1, "every verified discovery is answerable offline afterwards")
	s.Equal(fixtureNS("search-lumen"), after[0].Namespace)
}

// ------------------------------------------------------------- Bad paths ----

func (s *SearchSuite) TestDiscover_Start_BlankQueryIsRejected() {
	env := s.NewEnv(kit.WithProviders(newStubProvider(fixtureHost)))
	c := env.Client(s.T())

	testCases := []struct {
		name string
		body string
	}{
		{name: "empty string", body: `{"q":""}`},
		{name: "spaces", body: `{"q":"   "}`},
		{name: "tabs and newlines", body: `{"q":"\t\n"}`},
		{name: "absent field", body: `{}`},
		{name: "not json", body: `not json at all`},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp := c.Discover(tc.body)
			defer resp.Body.Close()
			s.Equal(http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func (s *SearchSuite) TestDiscover_Job_UnknownIDIsNotFound() {
	env := s.NewEnv(kit.WithProviders(newStubProvider(fixtureHost)))
	c := env.Client(s.T())

	testCases := []struct {
		name  string
		jobID string
	}{
		{name: "well formed uuid nobody started", jobID: "6f1a2b3c-4d5e-4f60-8a1b-2c3d4e5f6071"},
		{name: "not a uuid", jobID: "definitely-not-a-uuid"},
		{name: "numeric", jobID: "12345"},
		{name: "punctuation", jobID: "..."},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp := c.DiscoveryJob(tc.jobID)
			defer resp.Body.Close()
			s.Equal(http.StatusNotFound, resp.StatusCode,
				"an unreadable job id is a missing job, never a server error")
		})
	}
}

// TestDiscover_Provider_RateLimitIsDistinguishableFromNoResults is the bad path
// that matters most: "GitHub limited you, retry in 40s" must never render as
// "nothing found".
func (s *SearchSuite) TestDiscover_Provider_RateLimitIsDistinguishableFromNoResults() {
	limited := newHTTPProvider(s.T(), fixtureHost, respondStatus(
		http.StatusForbidden,
		http.Header{
			"X-Ratelimit-Remaining": []string{"0"},
			"Retry-After":           []string{"40"},
		},
	))
	env := s.NewEnv(kit.WithProviders(limited))
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)

	summary := s.waitForCompleted(tc, job.JobID)
	s.Zero(summary.Found)
	s.Zero(summary.Verified)

	outcome, ok := providerFor(summary, fixtureHost)
	s.Require().True(ok)
	s.False(outcome.OK, "a rate-limited host is not an ok host that found nothing")
	s.Equal(discovery.ReasonRateLimited, outcome.Reason)
	s.Equal(40, outcome.RetryAfter, "the client is told how long to wait")
}

func (s *SearchSuite) TestDiscover_Provider_RateLimitWithoutAHintReportsNoRetryAfter() {
	limited := newHTTPProvider(s.T(), fixtureHost, respondStatus(
		http.StatusTooManyRequests,
		http.Header{},
	))
	env := s.NewEnv(kit.WithProviders(limited))
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)

	summary := s.waitForCompleted(tc, job.JobID)
	outcome, ok := providerFor(summary, fixtureHost)
	s.Require().True(ok)
	s.False(outcome.OK)
	s.Equal(discovery.ReasonRateLimited, outcome.Reason)
	s.Zero(outcome.RetryAfter, "a host that gave no hint reports none")
}

func (s *SearchSuite) TestDiscover_Provider_EmptyResultIsAnOkHostThatFoundNothing() {
	empty := newHTTPProvider(s.T(), fixtureHost, respondJSON(`{"items":[]}`))
	env := s.NewEnv(kit.WithProviders(empty))
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)

	summary := s.waitForCompleted(tc, job.JobID)
	s.Zero(summary.Found, "finding nothing completes the job cleanly")
	s.Zero(summary.Verified)
	s.Zero(summary.Skipped)

	outcome, ok := providerFor(summary, fixtureHost)
	s.Require().True(ok)
	s.True(outcome.OK, "an empty result set is a working host, not a failure")
	s.Zero(outcome.Returned)
	s.Empty(outcome.Reason)
}

func (s *SearchSuite) TestDiscover_Provider_FailuresAreReportedPerHost() {
	testCases := []struct {
		name       string
		do         provider.DoFunc
		wantReason string
	}{
		{
			name:       "server error",
			do:         respondStatus(http.StatusInternalServerError, http.Header{}),
			wantReason: discovery.ReasonError,
		},
		{
			name:       "transport failure",
			do:         respondErr(fmt.Errorf("dial tcp: connection refused")),
			wantReason: discovery.ReasonError,
		},
		{
			name:       "timeout",
			do:         respondErr(context.DeadlineExceeded),
			wantReason: discovery.ReasonError,
		},
		{
			name:       "malformed json",
			do:         respondJSON(`{"items": [ this is not json`),
			wantReason: discovery.ReasonError,
		},
		{
			name:       "unauthorized",
			do:         respondStatus(http.StatusUnauthorized, http.Header{}),
			wantReason: discovery.ReasonUnauthorized,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			broken := newHTTPProvider(s.T(), fixtureHost, tc.do)
			env := s.NewEnv(kit.WithProviders(broken))
			client := env.TypedClient(s.T())

			job, status := client.Discover("widget")
			s.Require().Equal(http.StatusAccepted, status)

			summary := s.waitForCompleted(client, job.JobID)
			s.Equal(string(usecases.JobCompleted), summary.Status,
				"a broken host still completes the job")

			outcome, ok := providerFor(summary, fixtureHost)
			s.Require().True(ok)
			s.False(outcome.OK)
			s.Equal(tc.wantReason, outcome.Reason)
		})
	}
}

func (s *SearchSuite) TestDiscover_Provider_PartialSuccessKeepsTheHealthyHost() {
	healthy := newStubProvider(fixtureHost).answer("widget", candidateFor("search-widget-beta", 5))
	broken := newHTTPProvider(s.T(), secondHost, respondStatus(
		http.StatusForbidden,
		http.Header{"X-Ratelimit-Remaining": []string{"0"}},
	))
	env := s.NewEnv(kit.WithProviders(healthy, broken))
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)

	summary := s.waitForCompleted(tc, job.JobID)
	s.Equal(1, summary.Found, "one host down does not cost the other host's results")
	s.Equal(1, summary.Verified)
	s.Require().Len(summary.Providers, 2)

	good, ok := providerFor(summary, fixtureHost)
	s.Require().True(ok)
	s.True(good.OK)
	s.Equal(1, good.Returned)

	bad, ok := providerFor(summary, secondHost)
	s.Require().True(ok)
	s.False(bad.OK)
	s.Equal(discovery.ReasonRateLimited, bad.Reason)

	results, status := tc.Search("widget", kit.SearchParams{})
	s.Equal(http.StatusOK, status)
	s.Len(results, 1)
}

func (s *SearchSuite) TestDiscover_Provider_TwoHostsNamingOneRepoYieldOneResult() {
	first := newStubProvider(fixtureHost).answer("widget", candidateFor("search-widget-beta", 5))
	second := newStubProvider(secondHost).answer("widget", candidateForHost(secondHost, "search-widget-beta", 9))
	env := s.NewEnv(kit.WithProviders(first, second))
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)

	summary := s.waitForCompleted(tc, job.JobID)
	s.Equal(1, summary.Found, "a repository is one arrow however many hosts name it")
	s.Equal(1, summary.Verified)
}

func (s *SearchSuite) TestDiscover_Candidate_UnverifiableOnesAreSkipped() {
	testCases := []struct {
		name      string
		candidate provider.Candidate
	}{
		{name: "manifest 404s", candidate: missingCandidate("search-nothing-here")},
		{name: "manifest is not yaml", candidate: candidateFor("malformed", 3)},
		{name: "manifest fails the ruleset", candidate: candidateFor("invalid-ruleset", 3)},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			prov := newStubProvider(fixtureHost).answer(
				"widget",
				candidateFor("search-widget-beta", 1),
				tc.candidate,
			)
			env := s.NewEnv(kit.WithProviders(prov))
			client := env.TypedClient(s.T())

			job, status := client.Discover("widget")
			s.Require().Equal(http.StatusAccepted, status)

			summary := s.waitForCompleted(client, job.JobID)
			s.Equal(2, summary.Found)
			s.Equal(1, summary.Verified, "a verified result is always a real, compiled arrow")
			s.Equal(1, summary.Skipped, "an unverifiable candidate is counted, never emitted")

			results, _ := client.Search(tc.candidate.Name, kit.SearchParams{})
			s.Empty(results, "a skipped candidate never reaches the index")
		})
	}
}

// TestDiscover_Candidate_NoTargetForThisMachineStillVerifies pins the design:
// the compatible-OS list is a real filter carried on the result, not a reason to
// drop an arrow that happens to target another platform.
func (s *SearchSuite) TestDiscover_Candidate_NoTargetForThisMachineStillVerifies() {
	prov := newStubProvider(fixtureHost).answer("winonly", candidateFor("search-winonly", 2))
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	summary := s.runDiscovery(tc, "winonly", 1)
	s.Zero(summary.Skipped)

	results, status := tc.Search("winonly", kit.SearchParams{})
	s.Equal(http.StatusOK, status)
	s.Require().Len(results, 1)
	s.NotEmpty(results[0].CompatibleOS)
}

func (s *SearchSuite) TestDiscover_Providers_NoneConfiguredCompletesCleanly() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)

	summary := s.waitForCompleted(tc, job.JobID)
	s.Zero(summary.Found)
	s.Zero(summary.Verified)
	s.Zero(summary.Skipped)
	s.Empty(summary.Providers, "a pass that reaches no host still returns a well formed summary")
}

func (s *SearchSuite) TestDiscover_Job_ReadableAfterCompletion() {
	prov := newStubProvider(fixtureHost).answer("widget", candidateFor("search-widget-beta", 1))
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)
	s.waitForCompleted(tc, job.JobID)

	// The summary is read once, when the socket closes; the job outliving its
	// own stream is what makes that single read land.
	for range 3 {
		again, status := tc.DiscoveryJob(job.JobID)
		s.Equal(http.StatusOK, status)
		s.Equal(string(usecases.JobCompleted), again.Status)
		s.Equal(1, again.Verified)
	}
}
