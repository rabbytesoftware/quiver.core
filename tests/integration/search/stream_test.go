//go:build integration

package search_test

import (
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

// dialJob opens the result stream of a job and closes it on cleanup.
func (s *SearchSuite) dialJob(
	env *kit.Env,
	jobID string,
) *websocket.Conn {
	s.T().Helper()

	conn, err := env.Client(s.T()).DialDiscovery(jobID)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = conn.Close() })
	return conn
}

func (s *SearchSuite) TestStream_Results_DeliverRenderableArrows() {
	prov := newStubProvider(fixtureHost)
	gate := prov.gated(
		"widget",
		candidateFor("search-widget-beta", 128),
		candidateFor("search-widget-alpha", 4),
	)
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)

	conn := s.dialJob(env, job.JobID)
	env.WaitDiscoveryRegistered()
	close(gate)

	frames := readResults(s.T(), conn, 2)
	s.Require().Len(frames, 2)

	beta := frames[fixtureNS("search-widget-beta")]
	s.Equal("beta-widget", beta.Name, "a streamed result is renderable without a second round trip")
	s.Equal("A widget nobody curated", beta.Description)
	s.Equal([]string{"widget"}, beta.Tags)
	s.Equal([]string{fixtureBranch}, beta.Versions)
	s.NotEmpty(beta.CompatibleOS)
	s.Equal(models.ProvenanceSeen, beta.Provenance)
	s.Equal(128, beta.Stars)
	s.Equal(fixtureHost, beta.Source)
	s.False(beta.Installed)
	s.False(beta.Known)

	s.waitForCompleted(tc, job.JobID)
	requireQuiet(s.T(), conn)
}

// TestStream_Payload_CarriesOnlyResults is the regression guard on the one
// payload type contract. Every frame must decode into the result DTO with
// unknown fields rejected: a provider_error or done frame would fail here.
func (s *SearchSuite) TestStream_Payload_CarriesOnlyResults() {
	prov := newStubProvider(fixtureHost)
	// A pass with a failing host, a skipped candidate and a verified one has
	// every reason to want to say something other than "result".
	gate := prov.gated(
		"widget",
		candidateFor("search-widget-beta", 1),
		missingCandidate("search-nothing-here"),
		candidateFor("malformed", 1),
	)
	broken := newHTTPProvider(s.T(), secondHost, respondStatus(
		http.StatusForbidden,
		http.Header{"X-Ratelimit-Remaining": []string{"0"}, "Retry-After": []string{"40"}},
	))
	env := s.NewEnv(kit.WithProviders(prov, broken))
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)

	conn := s.dialJob(env, job.JobID)
	env.WaitDiscoveryRegistered()
	close(gate)

	frames := readResults(s.T(), conn, 1)
	s.Require().Contains(frames, fixtureNS("search-widget-beta"))

	summary := s.waitForCompleted(tc, job.JobID)
	s.Equal(2, summary.Skipped, "the skips happened, and none of them reached the stream")
	limited, ok := providerFor(summary, secondHost)
	s.Require().True(ok)
	s.False(limited.OK, "the refusal happened, and it did not reach the stream either")

	requireQuiet(s.T(), conn)
}

func (s *SearchSuite) TestStream_Known_ReportsAnArrowTheCatalogAlreadyHolds() {
	prov := newStubProvider(fixtureHost)
	gate := prov.gated("lumen", candidateFor("search-lumen", 3))
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	added := kit.NSFor("quiver-test/search-lumen", "v1")
	s.Require().Equal(http.StatusCreated, tc.Add(added))
	env.WaitForArrow(s.T(), added, catalogWait)

	job, status := tc.Discover("lumen")
	s.Require().Equal(http.StatusAccepted, status)

	conn := s.dialJob(env, job.JobID)
	env.WaitDiscoveryRegistered()
	close(gate)

	frames := readResults(s.T(), conn, 1)
	streamed := frames[fixtureNS("search-lumen")]
	s.True(streamed.Known, "an arrow the catalog already holds is reported as known")
	s.True(streamed.Installed)
	s.Empty(streamed.Provenance,
		"discovery cannot say which provenance the catalog recorded, so it claims none")
}

func (s *SearchSuite) TestStream_Jobs_NeverSeeEachOthersResults() {
	prov := newStubProvider(fixtureHost)
	widgetGate := prov.gated("widget", candidateFor("search-widget-beta", 1))
	lumenGate := prov.gated("lumen", candidateFor("search-lumen", 1))
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	widgetJob, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)
	lumenJob, status := tc.Discover("lumen")
	s.Require().Equal(http.StatusAccepted, status)

	// The broadcaster signals only its first subscriber, so the lumen client
	// registers first and is provably listening; the widget client proves its
	// own registration by receiving its result below.
	lumenConn := s.dialJob(env, lumenJob.JobID)
	env.WaitDiscoveryRegistered()
	widgetConn := s.dialJob(env, widgetJob.JobID)

	close(widgetGate)
	widgetFrames := readResults(s.T(), widgetConn, 1)
	s.Require().Contains(widgetFrames, fixtureNS("search-widget-beta"))
	s.waitForCompleted(tc, widgetJob.JobID)

	// The lumen client was listening throughout the widget pass, so the first
	// frame it ever receives being its own result is the cross-talk assertion.
	close(lumenGate)
	lumenFrames := readResults(s.T(), lumenConn, 1)
	s.Require().Contains(lumenFrames, fixtureNS("search-lumen"))
	s.waitForCompleted(tc, lumenJob.JobID)

	requireQuiet(s.T(), widgetConn)
	requireQuiet(s.T(), lumenConn)
}

func (s *SearchSuite) TestStream_LateSubscriber_GetsNothingAndTheResultsAreStillBanked() {
	prov := newStubProvider(fixtureHost).answer("widget", candidateFor("search-widget-beta", 1))
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)
	summary := s.waitForCompleted(tc, job.JobID)
	s.Require().Equal(1, summary.Verified)

	// The stream is live but the pass is over, so there is nothing left to send.
	conn := s.dialJob(env, job.JobID)
	requireQuiet(s.T(), conn)

	results, status := tc.Search("widget", kit.SearchParams{})
	s.Equal(http.StatusOK, status)
	s.Len(results, 1, "a client that arrived late still finds the arrow locally")
}

func (s *SearchSuite) TestStream_Disconnect_MidStreamLeavesTheDaemonHealthy() {
	prov := newStubProvider(fixtureHost)
	gate := prov.gated(
		"widget",
		candidateFor("search-widget-beta", 1),
		candidateFor("search-widget-alpha", 1),
		candidateFor("search-lumen", 1),
		candidateFor("search-lumendb", 1),
	)
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)

	conn := s.dialJob(env, job.JobID)
	env.WaitDiscoveryRegistered()
	close(gate)

	readResults(s.T(), conn, 1)
	s.Require().NoError(conn.Close())

	// Losing the last subscriber cancels the pass. It still completes, and
	// whatever it verified before the cancel is banked in the vault.
	summary := s.waitForCompleted(tc, job.JobID)
	s.GreaterOrEqual(summary.Verified, 1)
	s.LessOrEqual(summary.Verified, 4)

	results, status := tc.Search("widget", kit.SearchParams{})
	s.Equal(http.StatusOK, status)
	s.NotEmpty(results, "the daemon still answers after a client vanished")

	after, status := tc.Discover("lumen")
	s.Equal(http.StatusAccepted, status, "a later pass still starts")
	s.waitForCompleted(tc, after.JobID)
}

func (s *SearchSuite) TestStream_NoSubscriber_StillBanksTheResults() {
	prov := newStubProvider(fixtureHost).answer("widget", candidateFor("search-widget-beta", 1))
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("widget")
	s.Require().Equal(http.StatusAccepted, status)

	summary := s.waitForCompleted(tc, job.JobID)
	s.Equal(1, summary.Verified, "a pass nobody watched runs anyway")

	results, status := tc.Search("widget", kit.SearchParams{})
	s.Equal(http.StatusOK, status)
	s.Len(results, 1)
}
