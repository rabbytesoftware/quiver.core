//go:build integration

package selfupdate_test

import (
	"net/http"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

// A quiver.core row that another installed arrow depends on is not an
// "earlier build's" row: removing it on boot pulls a dependency out from
// under an installed dependent, bypassing the dependents guard every other
// removal honours (spec §9).
func (s *SelfUpdateSuite) TestAdversarial_BootKeepsACoreRowAnotherArrowDependsOn() {
	s.Repos.Set(selfNamespace, kit.BuildUpgradeRepo(s.T(), kit.ReadFixture(s.T(), "self-update/v1/arrow.yaml")))
	s.T().Cleanup(func() { s.Repos.Delete(selfNamespace) })
	const dependentKey = "quiver-test/adversarial-depends-on-core"
	dependentManifest := []byte(`schema: "arrow@v0"
metadata:
  name: quiver-test.adversarial-depends-on-core
  description: Depends on quiver.core
targets:
  "*":
    tools:
      - ` + selfNamespace + `@v1
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
`)
	s.Repos.Set(dependentKey, kit.BuildUpgradeRepo(s.T(), dependentManifest))
	s.T().Cleanup(func() { s.Repos.Delete(dependentKey) })

	home := s.T().TempDir()
	env1 := kit.BuildEnv(s.T(), s.Repos, s.CollectionRepos, home)
	tc1 := env1.TypedClient(s.T())
	dependent := kit.NSFor(dependentKey, "v1")
	s.Require().Equal(http.StatusCreated, tc1.Add(dependent))
	s.Require().Equal(http.StatusAccepted, tc1.Install(dependent, nil))
	env1.WaitForState(s.T(), dependent, domain.ArrowStateReady, 120*time.Second)
	s.Require().NotEqual(http.StatusOK, tc1.Remove(selfNamespace+"@v1"),
		"precondition: the API refuses to remove a row with dependents")
	env1.Close()

	env2 := kit.BuildEnv(s.T(), s.Repos, s.CollectionRepos, home, kit.WithBuild("v2", "bbb", "stable"))
	tc2 := env2.TypedClient(s.T())
	s.getDetail(tc2, selfNamespace+"@stable")

	_, status := tc2.GetDetail(selfNamespace + "@v1")
	s.Equal(http.StatusOK, status, "quiver.core@v1 is an installed arrow's dependency: booting core must not remove it")
}
