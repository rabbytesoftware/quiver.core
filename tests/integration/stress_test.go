//go:build integration

package integration_test

import (
	"fmt"
	"net/http"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func (s *StressSuite) TestStress_DeepChain() {
	env := s.newEnv()
	c := env.client(s.T())

	resp := c.Add(nsFor("dep-chain/a", "v1"))
	mustStatus(s.T(), resp, http.StatusCreated)
	resp = c.Install(nsFor("dep-chain/a", "v1"), nil)
	mustStatus(s.T(), resp, http.StatusAccepted)

	for _, letter := range "abcdefghijklmnopqrstuvwxyz" {
		ns := nsFor(fmt.Sprintf("dep-chain/%s", string(letter)), "v1")
		waitForState(s.T(), c, ns, domain.ArrowStateReady, 60*time.Second)
	}
}

func (s *StressSuite) TestStress_WideGraph() {
	env := s.newEnv()
	c := env.client(s.T())

	resp := c.Add(nsFor("dep-wide/root", "v1"))
	mustStatus(s.T(), resp, http.StatusCreated)
	resp = c.Install(nsFor("dep-wide/root", "v1"), nil)
	mustStatus(s.T(), resp, http.StatusAccepted)

	// Wait for root
	waitForState(s.T(), c, nsFor("dep-wide/root", "v1"), domain.ArrowStateReady, 60*time.Second)

	// Wait for all 15 deps
	for i := 1; i <= 15; i++ {
		ns := nsFor(fmt.Sprintf("dep-wide/dep-%02d", i), "v1")
		waitForState(s.T(), c, ns, domain.ArrowStateReady, 60*time.Second)
	}
}

func (s *StressSuite) TestStress_BulkSeed100() {
	env := s.newEnv()
	c := env.client(s.T())
	content := readFixture(s.T(), "tool-a/arrow.yaml")

	for i := 1; i <= 100; i++ {
		ns := fmt.Sprintf("quiver.test/quiver-test/tool-bulk-%02d@v1", i)
		resp := c.Seed(ns, content)
		resp.Body.Close() // don't assert status — some may conflict
	}

	start := time.Now()
	resp := c.List()
	elapsed := time.Since(start)

	mustStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()
	s.Less(elapsed, 500*time.Millisecond, "List with 100 arrows should respond in <500ms")
}

func (s *StressSuite) TestStress_RestartSurvival() {
	home := s.T().TempDir()

	env1 := s.newEnvWithHome(home)
	c1 := env1.client(s.T())
	resp := c1.Add(nsFor("quiver-test/tool-a", "v1"))
	mustStatus(s.T(), resp, http.StatusCreated)
	resp = c1.Install(nsFor("quiver-test/tool-a", "v1"), nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c1, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 15*time.Second)
	env1.Close()

	env2 := s.newEnvWithHome(home)
	c2 := env2.client(s.T())
	resp = c2.GetDetail(nsFor("quiver-test/tool-a", "v1"))
	var detail map[string]any
	decodeJSON(s.T(), resp, &detail) // decodeJSON closes body
	s.Equal(http.StatusOK, resp.StatusCode, "GetDetail after restart must return 200")
	// state should be ready (survived restart)
	data, _ := detail["data"].(map[string]any)
	state, _ := data["state"].(string)
	s.Equal(string(domain.ArrowStateReady), state, "state should survive service restart")
}
