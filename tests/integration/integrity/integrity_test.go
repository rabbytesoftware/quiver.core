//go:build integration

package integrity_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type IntegritySuite struct{ kit.IntegrationSuite }

func TestIntegrityIntegration(t *testing.T) {
	suite.Run(t, new(IntegritySuite))
}

// waitForRef blocks until InstalledRef is set. MarkInstalled is sent from the
// runtime's onEnd hook and reaches the read model through its own projection, so
// the ready transition the WebSocket watcher reports does not imply it yet.
func (s *IntegritySuite) waitForRef(tc *kit.TypedClient, ns string) dto.ArrowDetailDTO {
	s.T().Helper()

	return kit.WaitForDetail(
		s.T(), tc, ns, "installed_ref to be populated", 30*time.Second,
		func(detail dto.ArrowDetailDTO, _ int) bool {
			return detail.InstalledRef != ""
		},
	)
}

func (s *IntegritySuite) TestIntegrity_FieldsAfterInstall() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	detail := s.waitForRef(tc, ns)
	s.Equal("v1", detail.InstalledRef, "InstalledRef must be set after install")
	s.NotEmpty(detail.InstalledAt, "InstalledAt must be set after install")
	s.NotEqual("0001-01-01T00:00:00Z", detail.InstalledAt)
	s.Equal("v1", detail.Version)
	s.Equal(string(domain.ArrowStateReady), detail.State)
}

func (s *IntegritySuite) TestIntegrity_FieldsAfterUpgrade() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	nsV1 := kit.NSFor("quiver-test/versioned-update", "v1")
	nsV2 := kit.NSFor("quiver-test/versioned-update", "v2")

	s.Equal(http.StatusCreated, tc.Add(nsV1))
	s.Equal(http.StatusAccepted, tc.Install(nsV1, nil))
	env.WaitForState(s.T(), nsV1, domain.ArrowStateReady, 120*time.Second)

	detailV1 := s.waitForRef(tc, nsV1)
	s.Equal("v1", detailV1.InstalledRef)
	s.Equal("v1", detailV1.Version)

	s.Equal(http.StatusCreated, tc.Add(nsV2))
	s.Equal(http.StatusAccepted, tc.Install(nsV2, nil))
	env.WaitForState(s.T(), nsV2, domain.ArrowStateReady, 120*time.Second)

	detailV2 := s.waitForRef(tc, nsV2)
	s.Equal("v2", detailV2.InstalledRef, "v2 InstalledRef must be v2")
	s.Equal("v2", detailV2.Version, "v2 Version must be v2")

	// v1 fields must be unchanged.
	detailV1After, _ := tc.GetDetail(nsV1)
	s.Equal("v1", detailV1After.Version, "v1 Version must not be overwritten by v2 install")
}

func (s *IntegritySuite) TestIntegrity_FieldsAfterUninstall() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 120*time.Second)

	// GetDetail must still return 200 (arrow is in catalog, just not installed).
	detail, status := tc.GetDetail(ns)
	s.Equal(http.StatusOK, status, "GetDetail after uninstall must return 200, not 404")
	s.Equal(string(domain.ArrowStateAbsent), detail.State)
	// Manifest fields must survive uninstall.
	s.Equal("quiver-test.tool-a", detail.Name)
	s.Equal("v1", detail.Version)
}
