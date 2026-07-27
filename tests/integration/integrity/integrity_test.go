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

// waitForInstallStamp blocks until installed_at is set. MarkInstalled is sent
// from the runtime's onEnd hook and reaches the read model through its own
// projection, so the ready transition the WebSocket watcher reports does not
// imply it yet.
//
// installed_at is the whole of the stamp: the ref it was installed at is the one
// the namespace already names, so the detail DTO carries no second copy.
func (s *IntegritySuite) waitForInstallStamp(tc *kit.TypedClient, ns string) dto.ArrowDetailDTO {
	s.T().Helper()

	return kit.WaitForDetail(
		s.T(), tc, ns, "installed_at to be populated", 120*time.Second,
		func(detail dto.ArrowDetailDTO, _ int) bool {
			return detail.InstalledAt != ""
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

	detail := s.waitForInstallStamp(tc, ns)
	s.NotEmpty(detail.InstalledAt, "InstalledAt must be set after install")
	s.NotEqual("0001-01-01T00:00:00Z", detail.InstalledAt)
	s.Equal("v1", domain.Namespace(detail.Namespace).Ref())
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

	detailV1 := s.waitForInstallStamp(tc, nsV1)
	s.Equal("v1", domain.Namespace(detailV1.Namespace).Ref())

	s.Equal(http.StatusCreated, tc.Add(nsV2))
	s.Equal(http.StatusAccepted, tc.Install(nsV2, nil))
	env.WaitForState(s.T(), nsV2, domain.ArrowStateReady, 120*time.Second)

	detailV2 := s.waitForInstallStamp(tc, nsV2)
	s.Equal("v2", domain.Namespace(detailV2.Namespace).Ref(), "v2 must report the v2 ref")

	// v1 fields must be unchanged: which ref an install stamped is answered by
	// which aggregate carries the stamp, so v2's install must leave v1's alone.
	detailV1After, _ := tc.GetDetail(nsV1)
	s.Equal("v1", domain.Namespace(detailV1After.Namespace).Ref(), "v1 must not be overwritten by v2 install")
	s.Equal(detailV1.InstalledAt, detailV1After.InstalledAt, "v2's install must not restamp v1")
	s.Equal(string(domain.ArrowStateReady), detailV1After.State)
}

func (s *IntegritySuite) TestIntegrity_FieldsAfterUninstall() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
	s.waitForInstallStamp(tc, ns)

	s.Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 120*time.Second)

	// MarkUninstalled travels the same asynchronous projection MarkInstalled
	// does, so the absent transition does not imply the stamp is off yet.
	detail := kit.WaitForDetail(
		s.T(), tc, ns, "installed_at to be cleared", 120*time.Second,
		func(detail dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && detail.InstalledAt == ""
		},
	)

	// The entry stays in the catalog, naming the ref it is filed under; only the
	// claim that the ref is on disk goes away.
	s.Equal(string(domain.ArrowStateAbsent), detail.State)
	s.Empty(detail.InstalledAt, "an uninstalled arrow must carry no install stamp")
	// Manifest fields must survive uninstall.
	s.Equal("quiver-test.tool-a", detail.Name)
	s.Equal("v1", domain.Namespace(detail.Namespace).Ref())
}
