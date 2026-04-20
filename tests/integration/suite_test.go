//go:build integration

package integration_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

// IntegrationSuite is the shared base embedded by all suites.
type IntegrationSuite struct {
	suite.Suite
	repos fixtureRepos
}

func (s *IntegrationSuite) SetupSuite() {
	s.repos = buildFixtureRepos(s.T())
}

// Suite types — one per test file category.
type LifecycleSuite struct{ IntegrationSuite }
type DepsSuite struct{ IntegrationSuite }
type EdgeSuite struct{ IntegrationSuite }
type VersioningSuite struct{ IntegrationSuite }
type ConcurrencySuite struct{ IntegrationSuite }
type StressSuite struct{ IntegrationSuite }

func TestLifecycleIntegration(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(LifecycleSuite))
}

func TestDepsIntegration(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(DepsSuite))
}

func TestEdgeIntegration(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(EdgeSuite))
}

func TestVersioningIntegration(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(VersioningSuite))
}

func TestConcurrencyIntegration(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(ConcurrencySuite))
}

func TestStressIntegration(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(StressSuite))
}
