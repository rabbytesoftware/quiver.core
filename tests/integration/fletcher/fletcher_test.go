//go:build integration

package fletcher_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

const (
	toolFixture      = "acme/tool"
	widgetFixture    = "acme/widget"
	declaredFixture  = "quiver-test/tool-a"
	releasingFixture = "acme-releases/tool"
	brandedFixture   = "acme/branded"
	plainFixture     = "acme/plain"
	rollingFixture   = "acme/rolling"
	avatarURL        = "https://avatars.example.test/u/7?v=4"
	waitTimeout      = 120 * time.Second
)

func TestMain(m *testing.M) { kit.Main(m) }

type FletcherSuite struct{ kit.IntegrationSuite }

func TestFletcherIntegration(t *testing.T) {
	suite.Run(t, new(FletcherSuite))
}

// A stable-channel row of a repository with no ARROW.md installs the drafted
// manifest, follows a new release in place, and uninstalls cleanly.
func (s *FletcherSuite) TestFletcher_InferredArrowLifecycle() {
	storer := s.releasedFixture(releasingFixture, "v1.0.0")
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(releasingFixture, "stable")

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, waitTimeout)

	detail, status := tc.GetDetail(ns)
	s.Require().Equal(http.StatusOK, status)
	s.Require().Equal(string(domain.ArrowOriginInferred), detail.Origin)
	s.Require().NotNil(detail.Inference)
	s.Require().Equal("high", detail.Inference.Confidence)
	s.Equal("v1.0.0", detail.ResolvedRef)
	arrows, listStatus := tc.List()
	s.Require().Equal(http.StatusOK, listStatus)
	s.Require().Len(arrows, 1)
	s.Require().Equal(string(domain.ArrowOriginInferred), arrows[0].Origin)
	s.Require().Equal("high", arrows[0].Confidence)
	workdir := s.exposedDir(env, "tool", "tool v1.0.0")

	var released string
	s.Repos.Mutate(func() { released = kit.TagHead(s.T(), storer, "v1.1.0") })
	result, checkStatus := tc.CheckAvailable(ns)
	s.Require().Equal(http.StatusOK, checkStatus)
	s.Require().NotNil(result.Available)
	s.Equal(dto.AvailableDTO{Ref: "v1.1.0", Commit: released}, *result.Available)

	s.Require().Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodUpdate, nil))
	kit.WaitForDetail(s.T(), tc, ns, "the row advanced to v1.1.0", waitTimeout,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK &&
				d.ResolvedRef == "v1.1.0" &&
				d.Available == nil &&
				d.State == string(domain.ArrowStateReady)
		})
	s.Equal(workdir, s.exposedDir(env, "tool", "tool v1.1.0"), "an update reinstalls in place")
	s.Equal(http.StatusOK, tc.Execute(ns, domain.MethodUpdate, nil), "nothing is ahead any more")

	s.Require().Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, waitTimeout)
	_, err := os.Lstat(s.binLink(env, "tool"))
	s.ErrorIs(err, fs.ErrNotExist)
}

// Every release of the fixture shares one commit, so only the ref the pin
// names can say which release's assets the drafted manifest installs.
func (s *FletcherSuite) TestFletcher_PinnedReleaseDraftsItsOwnTag() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(toolFixture, "v1.0.0")

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, waitTimeout)
	s.exposedDir(env, "tool", "tool v1.0.0")

	s.Equal(http.StatusOK, tc.Execute(ns, domain.MethodUpdate, nil), "a pin has nothing ahead of it")
	s.exposedDir(env, "tool", "tool v1.0.0")
}

func (s *FletcherSuite) TestFletcher_LowConfidenceIsAMissingManifest() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(widgetFixture, "v1.0.0")

	s.Equal(http.StatusNotFound, tc.Add(ns))
	s.Require().Positive(host.Calls())
	callsAfterAdd := host.Calls()
	_, detailStatus := tc.GetDetail(ns)
	s.Equal(http.StatusNotFound, detailStatus)
	s.Equal(callsAfterAdd, host.Calls())
	arrows, listStatus := tc.List()
	s.Require().Equal(http.StatusOK, listStatus)
	s.Empty(arrows)
}

func (s *FletcherSuite) TestFletcher_DisabledLeavesManifestlessRepoNotFound() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(toolFixture, "v1.0.0")

	s.Equal(http.StatusNotFound, tc.Add(ns))
	_, detailStatus := tc.GetDetail(ns)
	s.Equal(http.StatusNotFound, detailStatus)
}

func (s *FletcherSuite) TestFletcher_DeclaredArrowNeverAsksTheHost() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(declaredFixture, "v1")

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	detail, status := tc.GetDetail(ns)
	s.Require().Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowOriginDeclared), detail.Origin)
	s.Nil(detail.Inference)
	s.Zero(host.Calls())
}

func (s *FletcherSuite) TestFletcher_DetailOfAnUncataloguedRepoCataloguesNothing() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())

	detail, status := tc.GetDetail(kit.NSFor(toolFixture, "v1.0.0"))
	s.Require().Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowOriginInferred), detail.Origin)
	s.Require().NotNil(detail.Inference)
	s.Equal("high", detail.Inference.Confidence)

	arrows, listStatus := tc.List()
	s.Require().Equal(http.StatusOK, listStatus)
	s.Empty(arrows)
}

func (s *FletcherSuite) TestFletcher_AddedArrowCarriesMetadataLikeAHandWrittenOne() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	branded := kit.NSFor(brandedFixture, "v1.0.0")
	plain := kit.NSFor(plainFixture, "v1.0.0")

	s.Require().Equal(http.StatusCreated, tc.Add(branded))
	s.Require().Equal(http.StatusCreated, tc.Add(plain))

	arrows, status := tc.List()
	s.Require().Equal(http.StatusOK, status)
	byName := map[string]dto.ArrowListItemDTO{}
	for _, item := range arrows {
		byName[item.Name] = item
	}
	s.Require().Len(byName, 2)
	s.Equal("Authored by the maintainers", byName["branded"].Description)
	s.Contains(byName["branded"].Media.Icon, "/assets/logo.png")
	s.Equal("Plain description", byName["plain"].Description)
	s.Equal(avatarURL, byName["plain"].Media.Icon)
	s.Empty(byName["plain"].Media.Banner)
}

func (s *FletcherSuite) TestFletcher_RollingPointerRefInstallsDespiteUnreleasedStableTag() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	tip := kit.NSFor(rollingFixture, "tip")

	s.Require().Equal(http.StatusCreated, tc.Add(tip))
	s.Require().Equal(http.StatusAccepted, tc.Install(tip, nil))
	env.WaitForState(s.T(), tip, domain.ArrowStateReady, waitTimeout)

	detail, status := tc.GetDetail(tip)
	s.Require().Equal(http.StatusOK, status)
	s.False(detail.Outdated)
	s.Nil(detail.Available)
}

func (s *FletcherSuite) TestFletcher_DiscoveredArrowServesDetailAddAndInstallFromTheCache() {
	host := s.newHost()
	env := s.NewEnv(
		kit.WithFletcher(host.Lookup),
		kit.WithProviders(&discoveredProvider{fixtures: []string{toolFixture}}),
	)
	tc := env.TypedClient(s.T())
	bare := "quiver.test/" + toolFixture

	job, status := tc.Discover("tool")
	s.Require().Equal(http.StatusAccepted, status)
	s.Require().Eventually(func() bool {
		got, jobStatus := tc.DiscoveryJob(job.JobID)
		return jobStatus == http.StatusOK && got.Status == string(usecases.JobCompleted) && got.Verified == 1
	}, waitTimeout, 10*time.Millisecond)
	callsAfterDiscovery := host.Calls()
	s.Require().Positive(callsAfterDiscovery)

	detail, status := tc.GetDetail(bare)
	s.Require().Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowOriginInferred), detail.Origin)
	manifestStatus, manifest := tc.GetSub(bare, "manifest")
	s.Require().Equal(http.StatusOK, manifestStatus)
	readmeStatus, _ := tc.GetSub(bare, "readme")
	s.Require().Equal(http.StatusOK, readmeStatus)
	s.Equal(callsAfterDiscovery, host.Calls(), "opening the details of a discovered arrow builds nothing again")

	s.Require().Equal(http.StatusCreated, tc.Add(bare))
	installed := kit.NSFor(toolFixture, "stable")
	s.Require().Equal(http.StatusAccepted, tc.Install(installed, nil))
	env.WaitForState(s.T(), installed, domain.ArrowStateReady, waitTimeout)
	s.Equal(callsAfterDiscovery, host.Calls(), "add and install reuse the build discovery cached")
	added, status := tc.GetDetail(installed)
	s.Require().Equal(http.StatusOK, status)
	s.Equal("v1.1.0", added.ResolvedRef, "the add records the release the preview showed")

	addedStatus, addedManifest := tc.GetSub(installed, "manifest")
	s.Require().Equal(http.StatusOK, addedStatus)
	s.Equal(s.targetsOf(manifest), s.targetsOf(addedManifest))
}

// Reusing discovery's build never pins a row to it: once its tag moves, the
// update reads the release at the new commit from the host.
func (s *FletcherSuite) TestFletcher_ReusedBuildOfAMovedTagIsBuiltAgain() {
	storer := s.releasedFixture(releasingFixture, "v1.0.0")
	host := s.newHost()
	env := s.NewEnv(
		kit.WithFletcher(host.Lookup),
		kit.WithProviders(&discoveredProvider{fixtures: []string{releasingFixture}}),
	)
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(releasingFixture, "stable")

	job, status := tc.Discover("tool")
	s.Require().Equal(http.StatusAccepted, status)
	s.Require().Eventually(func() bool {
		got, jobStatus := tc.DiscoveryJob(job.JobID)
		return jobStatus == http.StatusOK && got.Status == string(usecases.JobCompleted) && got.Verified == 1
	}, waitTimeout, 10*time.Millisecond)
	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, waitTimeout)
	callsBeforeMove := host.Calls()

	var moved string
	s.Repos.Mutate(func() { moved = kit.MoveTagToNewCommit(s.T(), storer, "v1.0.0") })
	s.Require().Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodUpdate, nil))
	kit.WaitForDetail(s.T(), tc, ns, "the row advanced to the moved commit", waitTimeout,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.InstalledCommit == moved && d.State == string(domain.ArrowStateReady)
		})
	s.Greater(host.Calls(), callsBeforeMove, "the moved tag's release is read from the host again")
}

// A branch of a repository with no manifest publishes no release, so there
// is nothing to install that follows it: the add answers not found and
// catalogues nothing, while its release tags stay installable.
func (s *FletcherSuite) TestFletcher_BranchOfAManifestlessRepoIsNotFound() {
	s.releasedFixture(releasingFixture, "v1.0.0")
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusNotFound, tc.Add(kit.NSFor(releasingFixture, "master")))
	_, detailStatus := tc.GetDetail(kit.NSFor(releasingFixture, "master"))
	s.Equal(http.StatusNotFound, detailStatus)
	arrows, status := tc.List()
	s.Require().Equal(http.StatusOK, status)
	s.Empty(arrows)

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor(releasingFixture, "v1.0.0")))
}

func (s *FletcherSuite) targetsOf(
	body []byte,
) any {
	var envelope struct {
		Data struct {
			Manifest struct {
				Namespace string `json:"namespace"`
				Targets   any    `json:"targets"`
			} `json:"manifest"`
		} `json:"data"`
	}
	s.Require().NoError(json.Unmarshal(body, &envelope))
	s.Require().NotEmpty(envelope.Data.Manifest.Targets)
	return envelope.Data.Manifest.Targets
}

func (s *FletcherSuite) newHost() kit.FakeHost {
	return kit.NewFakeHost(s.T(), map[string]kit.HostRepo{
		"quiver.test/" + toolFixture: {
			Description: "A tool for integration tests",
			Readme:      "# tool\n\nA tool for integration tests.\n",
			Binary:      "tool",
			Asset:       "tool_{tag}_{os}_{arch}.tar.gz",
			Tags:        []string{"v1.0.0", "v1.1.0"},
		},
		"quiver.test/" + releasingFixture: {
			Description: "A tool for integration tests",
			Readme:      "# tool\n\nA tool for integration tests.\n",
			Binary:      "tool",
			Asset:       "tool_{tag}_{os}_{arch}.tar.gz",
			Tags:        []string{"v1.0.0", "v1.1.0"},
		},
		"quiver.test/" + widgetFixture: {
			Description: "A widget for integration tests",
			Readme:      "# widget\n",
			Binary:      "gadget-cli",
			Asset:       "gadget-cli_{os}_{arch}.tar.gz",
			Tags:        []string{"v1.0.0"},
		},
		"quiver.test/" + brandedFixture: {
			Description:    "Scraped, never used",
			APIDescription: "Authored by the maintainers",
			AvatarURL:      avatarURL,
			Readme:         "# branded\n",
			Binary:         "branded",
			Asset:          "branded_{tag}_{os}_{arch}.tar.gz",
			Tags:           []string{"v1.0.0"},
			Files:          map[string][]byte{"assets/logo.png": squarePNG(s.T(), 256)},
		},
		"quiver.test/" + plainFixture: {
			Description: "Plain description",
			AvatarURL:   avatarURL,
			Readme:      "# plain\n",
			Binary:      "plain",
			Asset:       "plain_{tag}_{os}_{arch}.tar.gz",
			Tags:        []string{"v1.0.0"},
		},
		"quiver.test/" + rollingFixture: {
			Description: "Publishes only a rolling pre-release",
			Readme:      "# rolling\n",
			Binary:      "rolling",
			Asset:       "rolling_{tag}_{os}_{arch}.tar.gz",
			Tags:        []string{"tip"},
		},
		"quiver.test/" + declaredFixture: {
			Description: "A repo that ships its own ARROW.md",
			Readme:      "# tool-a\n",
			Binary:      "tool-a",
			Asset:       "tool-a_{tag}_{os}_{arch}.tar.gz",
			Tags:        []string{"v1"},
		},
	})
}

// releasedFixture registers a manifestless repository owned by one test, since
// the test cuts a release on it.
func (s *FletcherSuite) releasedFixture(
	key string,
	tag string,
) *memory.Storage {
	storer := kit.BuildManifestlessRepo(s.T(), tag)
	s.Repos.Set(key, storer)
	s.T().Cleanup(func() { s.Repos.Delete(key) })
	return storer
}

func (s *FletcherSuite) binLink(
	env *kit.Env,
	name string,
) string {
	bin, err := paths.BinAt(env.Home)
	s.Require().NoError(err)
	return filepath.Join(bin, name)
}

func (s *FletcherSuite) exposedDir(
	env *kit.Env,
	name string,
	want string,
) string {
	link := s.binLink(env, name)
	info, err := os.Lstat(link)
	s.Require().NoError(err)
	s.Require().NotZero(info.Mode() & fs.ModeSymlink)
	target, err := filepath.EvalSymlinks(link)
	s.Require().NoError(err)
	s.Run("executes "+want, func() {
		if runtime.GOOS == "windows" {
			s.T().Skip("windows exposes the command folder on the user Path, not an executable symlink")
		}
		out, err := exec.Command(link).Output()
		s.Require().NoError(err)
		s.Equal(want+"\n", string(out))
	})
	return filepath.Dir(target)
}

func squarePNG(
	t *testing.T,
	side int,
) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, side, side))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}
