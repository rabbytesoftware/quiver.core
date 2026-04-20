//go:build integration

package integration_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type IntegrationSuite struct {
	suite.Suite
	repos fixtureRepos
}

func (s *IntegrationSuite) SetupSuite() {
	s.repos = buildFixtureRepos(s.T())
}

func TestIntegration(t *testing.T) {
	suite.Run(t, new(IntegrationSuite))
}
