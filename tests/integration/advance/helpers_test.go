//go:build integration

package advance_test

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/storage/memory"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

// wait is a failure deadline, never a delay: see tests/kit/waiters.go.
const wait = 120 * time.Second

// Files the rolling-tool and stable-tool fixtures write into their workdir.
const (
	installMarker = "install-marker"
	updateRefs    = "update-refs"
	holdFile      = "hold"
	releaseFile   = "release"
)

// fixture is a repo owned by one test: its tags move, so it is never shared.
type fixture struct {
	key    string
	storer *memory.Storage
}

func (f fixture) ns(selector string) string {
	return kit.NSFor(f.key, selector)
}

// newFixture registers a repo whose only commit is content tagged tag, under a
// key no other test uses.
func (s *AdvanceSuite) newFixture(name, tag string, content []byte) fixture {
	testName := s.T().Name()
	slug := strings.ToLower(strings.ReplaceAll(testName[strings.LastIndex(testName, "/")+1:], "_", "-"))
	key := "quiver-test/" + name + "-" + slug

	storer := kit.BuildTaggedRepo(s.T(), tag, content)
	s.Repos.Set(key, storer)
	s.T().Cleanup(func() { s.Repos.Delete(key) })
	return fixture{key: key, storer: storer}
}

func (s *AdvanceSuite) moveTag(f fixture, tag string) string {
	var commit string
	s.Repos.Mutate(func() { commit = kit.MoveTagToNewCommit(s.T(), f.storer, tag) })
	return commit
}

// publish cuts a new release tag. Each release needs distinct bytes, since a
// commit identical to its parent is refused, so the tag is appended to
// content as a YAML comment.
func (s *AdvanceSuite) publish(f fixture, tag string, content []byte) string {
	var commit string
	s.Repos.Mutate(func() {
		kit.AddTaggedCommitToRepo(s.T(), f.storer, tag, release(content, tag))
		commit = kit.TagCommit(s.T(), f.storer, tag)
	})
	return commit
}

func (s *AdvanceSuite) tagCommit(f fixture, tag string) string {
	var commit string
	s.Repos.Mutate(func() { commit = kit.TagCommit(s.T(), f.storer, tag) })
	return commit
}

func release(content []byte, tag string) []byte {
	return []byte(string(content) + "\n# " + tag + "\n")
}

func (s *AdvanceSuite) install(env *kit.Env, tc *kit.TypedClient, ns string) {
	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, wait)
}

func (s *AdvanceSuite) detail(tc *kit.TypedClient, ns string) dto.ArrowDetailDTO {
	detail, status := tc.GetDetail(ns)
	s.Require().Equal(http.StatusOK, status, "GET /arrow/%s", ns)
	return detail
}

// checkAvailable runs the explicit check (PATCH /arrow/:ns) and returns what
// it found ahead of the row, nil when current.
func (s *AdvanceSuite) checkAvailable(tc *kit.TypedClient, ns string) *dto.AvailableDTO {
	result, status := tc.CheckAvailable(ns)
	s.Require().Equal(http.StatusOK, status, "PATCH /arrow/%s", ns)
	return result.Available
}

func (s *AdvanceSuite) update(tc *kit.TypedClient, ns string) {
	s.Require().Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodUpdate, nil))
}

// waitAdvanced waits until the row stands at commit with nothing ahead of it
// and its runtime back at ready: the update's bracket committed and cleared
// the badge.
func (s *AdvanceSuite) waitAdvanced(tc *kit.TypedClient, ns, commit string) dto.ArrowDetailDTO {
	return kit.WaitForDetail(s.T(), tc, ns, "the row advanced to "+commit, wait,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK &&
				d.InstalledCommit == commit &&
				d.Available == nil &&
				d.State == string(domain.ArrowStateReady)
		},
	)
}

func (s *AdvanceSuite) workDir(env *kit.Env, ns string) string {
	dir, err := env.Vault.WorkDir(context.Background(), domain.Namespace(ns))
	s.Require().NoError(err)
	return dir
}

// workFile reads a file the fixture's steps wrote into ns's workdir.
func (s *AdvanceSuite) workFile(env *kit.Env, ns, name string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(s.workDir(env, ns), name)) // #nosec G304 -- a temp workdir this test owns
	if errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	s.Require().NoError(err)
	return string(data), true
}

func (s *AdvanceSuite) touchWorkFile(env *kit.Env, ns, name string) {
	s.Require().NoError(os.WriteFile(filepath.Join(s.workDir(env, ns), name), nil, 0o600))
}

// updateRuns lists the ${REF} of every update step run in ns's workdir, in
// order.
func (s *AdvanceSuite) updateRuns(env *kit.Env, ns string) []string {
	content, ok := s.workFile(env, ns, updateRefs)
	if !ok {
		return nil
	}
	return strings.Fields(content)
}

// detailPoller reads GET /arrow/:ns until stopped and keeps every answer that
// was not a 200, proving an identity stays readable across a whole sequence.
type detailPoller struct {
	done   chan struct{}
	result chan pollResult
	once   sync.Once
}

type pollResult struct {
	polls int
	bad   []int
}

func (s *AdvanceSuite) pollDetail(env *kit.Env, ns string) *detailPoller {
	p := &detailPoller{done: make(chan struct{}), result: make(chan pollResult, 1)}
	s.T().Cleanup(p.halt)
	client := env.HTTPClient(30 * time.Second)
	target := env.URL + "/v0/arrow/" + url.PathEscape(ns)

	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		var r pollResult
		for {
			r.polls++
			resp, err := client.Get(target)
			switch {
			case err != nil:
				r.bad = append(r.bad, 0)
			default:
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					r.bad = append(r.bad, resp.StatusCode)
				}
			}
			select {
			case <-p.done:
				p.result <- r
				return
			case <-ticker.C:
			}
		}
	}()
	return p
}

// stop ends polling and reports how many reads ran and every non-200 answer.
func (p *detailPoller) stop() (int, []int) {
	p.halt()
	r := <-p.result
	return r.polls, r.bad
}

func (p *detailPoller) halt() {
	p.once.Do(func() { close(p.done) })
}
