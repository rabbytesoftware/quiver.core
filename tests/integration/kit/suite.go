package kit

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

// IntegrationSuite is the shared base embedded by all integration suite types.
// Each suite calls SetupSuite once to build in-memory fixture repos.
type IntegrationSuite struct {
	suite.Suite
	Repos FixtureRepos
}

// SetupSuite builds all in-memory git fixture repos once per suite run.
func (s *IntegrationSuite) SetupSuite() {
	s.Repos = BuildFixtureRepos(s.T())
}

// Main is called by each suite package's TestMain.
// It silences slog and sets gin to test mode before running tests.
func Main(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}
