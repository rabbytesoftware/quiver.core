//go:build integration

package console_test

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(
	m *testing.M,
) {
	kit.Main(m)
}

type ConsoleSuite struct {
	kit.IntegrationSuite
}

func TestConsoleIntegration(
	t *testing.T,
) {
	suite.Run(t, new(ConsoleSuite))
}

type frame struct {
	Type  string `json:"type"`
	Data  string `json:"data"`
	Code  int    `json:"code"`
	Error string `json:"error"`
}

func (s *ConsoleSuite) exec(
	c *kit.Client,
	line string,
) (frame, string) {
	resp := c.ConsoleExec(line)
	defer resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode, line)

	var exit frame
	var text strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var f frame
		s.Require().NoError(json.Unmarshal(scanner.Bytes(), &f))
		text.WriteString(f.Data)
		exit = f
	}
	return exit, text.String()
}

func (s *ConsoleSuite) TestConsole_Exec_RunsTheDaemonsOwnCLIAgainstTheRealDaemon() {
	env := s.NewEnv()
	c := env.Client(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	added, _ := s.exec(c, "arrow add "+ns)
	s.Equal(0, added.Code, added.Error)
	env.WaitForArrow(s.T(), ns, 30*time.Second)
	_, listing := s.exec(c, "arrow list")
	s.Contains(listing, "tool-a")
	s.NotContains(listing, "\x1b[", "the console renders plain text")

	installed, _ := s.exec(c, "install "+ns)
	s.Equal(0, installed.Code, installed.Error)
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	refused, _ := s.exec(c, "arrow remove "+ns)
	s.Equal(2, refused.Code)
	s.Contains(refused.Error, "--yes", "confirmations answer no")
}

func (s *ConsoleSuite) TestConsole_Exec_RefusesWhatTheConsoleMustNotRun() {
	c := s.NewEnv().Client(s.T())

	for line, want := range map[string]int{
		"daemon":                          http.StatusForbidden,
		"self-update /tmp/x":              http.StatusForbidden,
		"context list":                    http.StatusForbidden,
		"arrow seed x --file /etc/passwd": http.StatusForbidden,
		"help":                            http.StatusForbidden,
		"ps; daemon":                      http.StatusBadRequest,
	} {
		resp := c.ConsoleExec(line)
		resp.Body.Close()
		s.Equal(want, resp.StatusCode, line)
	}
}

func (s *ConsoleSuite) TestConsole_Logs_StreamTheDaemonsOwnAuditRecord() {
	ring := logring.New()
	previous := slog.Default()
	slog.SetDefault(slog.New(ring.Wrap(slog.NewTextHandler(io.Discard, nil))))
	defer slog.SetDefault(previous)
	c := s.NewEnv(kit.WithLogRing(ring)).Client(s.T())
	conn, err := c.DialConsoleLogs("")
	s.Require().NoError(err)
	defer conn.Close()
	s.Require().NoError(conn.SetReadDeadline(time.Now().Add(30 * time.Second)))
	var f map[string]any
	for f == nil || f["type"] != "ready" {
		s.Require().NoError(conn.ReadJSON(&f))
	}

	s.exec(c, "ps")

	for f["msg"] != "exec" {
		s.Require().NoError(conn.ReadJSON(&f))
	}
	s.Equal("console", f["component"])
	s.Equal("unix", f["fields"].(map[string]any)["device"])
}
