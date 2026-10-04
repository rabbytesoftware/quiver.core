//go:build integration

package kit

import (
	"fmt"
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
	Repos           *FixtureRepos
	CollectionRepos *FixtureRepos
}

// SetupSuite builds all in-memory git fixture repos once per suite run.
func (s *IntegrationSuite) SetupSuite() {
	s.Repos = BuildFixtureRepos(s.T())
	s.CollectionRepos = BuildFixtureCollectionRepos(s.T(), s.Repos)
}

// Main is called by each suite package's TestMain.
// It silences slog, sets gin to test mode and gives the process its own empty
// Quiver home (so the developer's real config never leaks in) before running
// tests.
func Main(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	gin.SetMode(gin.TestMode)

	cleanup, err := isolateHome()
	if err != nil {
		fmt.Fprintln(os.Stderr, "kit: isolate quiver home:", err)
		os.Exit(1)
	}

	code := m.Run()
	cleanup()
	os.Exit(code)
}
