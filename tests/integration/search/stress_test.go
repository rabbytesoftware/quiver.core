//go:build integration

package search_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

// stressHTTPTimeout bounds one request inside a stress goroutine. It is a
// safety net against a hung socket, never a performance assertion.
const stressHTTPTimeout = 60 * time.Second

// bulkManifest is a valid arrow whose metadata is distinctive enough to be
// matched by a single query across the whole bulk set.
func bulkManifest(name string) []byte {
	return []byte(fmt.Sprintf(`schema: "arrow@v0"
metadata:
  name: %s
  description: Searchbulk fixture arrow
  tags:
    - searchbulk
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: echo installed
          title: Install
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
`, name))
}

// registerBulkRepos builds count in-memory fixture repos and returns a
// candidate for each, so a stress pass can push a large set through verify.
func (s *SearchSuite) registerBulkRepos(count int) []provider.Candidate {
	s.T().Helper()

	candidates := make([]provider.Candidate, 0, count)
	for i := range count {
		fixture := fmt.Sprintf("search-bulk-%03d", i)
		name := fmt.Sprintf("searchbulk%03d", i)
		s.Repos.Set("quiver-test/"+fixture, kit.BuildUpgradeRepo(s.T(), bulkManifest(name)))
		candidates = append(candidates, candidateFor(fixture, i))
	}
	return candidates
}

// searchProbe is one Lane A read taken from a stress goroutine. Errors are
// collected rather than asserted in place: only the test goroutine may fail the
// test.
type searchProbe struct {
	status  int
	results []apidto.SearchResultDTO
	err     error
}

// probeSearch issues one GET /v0/search without touching *testing.T.
func probeSearch(
	client *http.Client,
	baseURL string,
	query string,
) searchProbe {
	target := baseURL + "/v0/search?q=" + url.QueryEscape(query) + "&limit=100"

	resp, err := client.Get(target) //nolint:noctx // the client carries the timeout
	if err != nil {
		return searchProbe{err: err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return searchProbe{status: resp.StatusCode, err: err}
	}

	var env struct {
		Data []apidto.SearchResultDTO `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return searchProbe{status: resp.StatusCode, err: fmt.Errorf("decode %s: %w", body, err)}
	}
	return searchProbe{status: resp.StatusCode, results: env.Data}
}

// TestStress_Discovery_ManyConcurrentJobs runs one host against many
// simultaneous passes. Every pass must complete with its own correct summary,
// and the repeated writes of the same rows must leave one result per namespace.
func (s *SearchSuite) TestStress_Discovery_ManyConcurrentJobs() {
	const jobs = 16

	prov := newStubProvider(fixtureHost).answer(
		"widget",
		candidateFor("search-widget-beta", 3),
		candidateFor("search-widget-alpha", 5),
	)
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	started := make([]apidto.DiscoveryJobStartedDTO, 0, jobs)
	ids := make(map[string]struct{}, jobs)
	for range jobs {
		job, status := tc.Discover("widget")
		s.Require().Equal(http.StatusAccepted, status)
		s.Require().NotContains(ids, job.JobID, "job ids must be unique")
		ids[job.JobID] = struct{}{}
		started = append(started, job)
	}

	for _, job := range started {
		summary := s.waitForCompleted(tc, job.JobID)
		s.Equal(2, summary.Found, "job %s", job.JobID)
		s.Equal(2, summary.Verified, "job %s", job.JobID)
		s.Zero(summary.Skipped, "job %s", job.JobID)
	}

	s.Equal(jobs, prov.searches())

	results, status := tc.Search("widget", kit.SearchParams{})
	s.Equal(http.StatusOK, status)
	s.Len(results, 2, "concurrent upserts of the same rows must not duplicate them")
	for _, r := range results {
		s.Equal([]string{fixtureBranch}, r.Versions)
	}
}

// TestStress_Search_ReadsDuringDiscoveryAreNeverTorn hammers Lane A while a
// discovery pass writes to the vault. A reader may legitimately see fewer rows
// than the pass will eventually produce, but never a half-written one.
func (s *SearchSuite) TestStress_Search_ReadsDuringDiscoveryAreNeverTorn() {
	const readers = 8

	candidates := s.registerBulkRepos(60)
	prov := newStubProvider(fixtureHost).answer("searchbulk", candidates...)
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	client := env.HTTPClient(stressHTTPTimeout)
	stop := make(chan struct{})

	var mu sync.Mutex
	var probes []searchProbe

	var wg sync.WaitGroup
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				probe := probeSearch(client, env.URL, "searchbulk")
				mu.Lock()
				probes = append(probes, probe)
				mu.Unlock()
			}
		}()
	}

	job, status := tc.Discover("searchbulk")
	s.Require().Equal(http.StatusAccepted, status)
	summary := s.waitForCompleted(tc, job.JobID)
	close(stop)
	wg.Wait()

	s.Require().Equal(len(candidates), summary.Verified)

	mu.Lock()
	defer mu.Unlock()
	s.Require().NotEmpty(probes, "the readers must have run alongside the pass")
	for i, probe := range probes {
		s.Require().NoError(probe.err, "probe %d", i)
		s.Require().Equal(http.StatusOK, probe.status, "probe %d", i)
		for _, row := range probe.results {
			s.NotEmpty(row.Namespace, "probe %d returned a row with no namespace", i)
			s.NotEmpty(row.Name, "probe %d returned %s with no name", i, row.Namespace)
			s.NotEmpty(row.Versions, "probe %d returned %s with no version", i, row.Namespace)
			s.NotEmpty(row.CompatibleOS, "probe %d returned %s with no compatible os", i, row.Namespace)
			s.Equal([]string{"searchbulk"}, row.Tags, "probe %d returned %s with torn tags", i, row.Namespace)
		}
	}

	final, status := tc.Search("searchbulk", kit.SearchParams{Limit: "100"})
	s.Equal(http.StatusOK, status)
	s.Len(final, len(candidates))
}

// TestStress_Discovery_LargeCandidateSet pushes a set well past the fetch
// concurrency bound through verify, mixing in candidates that cannot be proven.
func (s *SearchSuite) TestStress_Discovery_LargeCandidateSet() {
	const (
		valid   = 100
		invalid = 20
	)

	candidates := s.registerBulkRepos(valid)
	for i := range invalid {
		candidates = append(candidates, missingCandidate(fmt.Sprintf("search-absent-%03d", i)))
	}

	prov := newStubProvider(fixtureHost).answer("searchbulk", candidates...)
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	job, status := tc.Discover("searchbulk")
	s.Require().Equal(http.StatusAccepted, status)

	summary := s.waitForCompleted(tc, job.JobID)
	s.Equal(valid+invalid, summary.Found)
	s.Equal(valid, summary.Verified)
	s.Equal(invalid, summary.Skipped)
	s.Equal(summary.Found, summary.Verified+summary.Skipped,
		"every candidate is accounted for exactly once")

	results, status := tc.Search("searchbulk", kit.SearchParams{Limit: "100"})
	s.Equal(http.StatusOK, status)
	s.Len(results, 100, "the limit caps the answer, not the index")
}

// TestStress_Add_ConcurrentRefsOfOneNamespace adds several refs of the same
// arrow at once. Each ref is its own aggregate, so all of them must land and
// collapse onto one result carrying every version.
func (s *SearchSuite) TestStress_Add_ConcurrentRefsOfOneNamespace() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	client := env.HTTPClient(stressHTTPTimeout)

	refs := []string{"v1", "v2"}
	statuses := make([]int, len(refs))
	errs := make([]error, len(refs))

	var wg sync.WaitGroup
	for i, ref := range refs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			target := env.URL + "/v0/arrow/" + url.PathEscape(kit.NSFor("quiver-test/search-multi", ref))
			resp, err := client.Post(target, "application/json", nil) //nolint:noctx // the client carries the timeout
			if err != nil {
				errs[i] = err
				return
			}
			defer resp.Body.Close()
			statuses[i] = resp.StatusCode
		}()
	}
	wg.Wait()

	for i, ref := range refs {
		s.Require().NoError(errs[i], "add %s", ref)
		s.Equal(http.StatusCreated, statuses[i], "add %s", ref)
	}

	s.Require().Eventually(func() bool {
		results, _ := tc.Search("multiverse", kit.SearchParams{})
		return len(results) == 1 && len(results[0].Versions) == len(refs)
	}, catalogWait, 20*time.Millisecond)

	results, status := tc.Search("multiverse", kit.SearchParams{})
	s.Equal(http.StatusOK, status)
	s.Require().Len(results, 1)
	s.Equal(refs, results[0].Versions)
}
