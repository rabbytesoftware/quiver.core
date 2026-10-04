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

	"github.com/rabbytesoftware/quiver.core/internal/console/logring"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type ConsoleSuite struct{ kit.IntegrationSuite }

func TestConsoleIntegration(t *testing.T) {
	suite.Run(t, new(ConsoleSuite))
}

type frame struct {
	Type   string `json:"type"`
	Stream string `json:"stream"`
	Data   string `json:"data"`
	Code   int    `json:"code"`
	Error  string `json:"error"`
}

func (s *ConsoleSuite) exec(
	c *kit.Client,
	line string,
) (frame, string) {
	resp := c.ConsoleExec(line)
	defer resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode, line)

	var out strings.Builder
	var last frame
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var f frame
		s.Require().NoError(json.Unmarshal(scanner.Bytes(), &f))
		if f.Type == "out" {
			out.WriteString(f.Data)
		}
		last = f
	}
	return last, out.String()
}

func (s *ConsoleSuite) TestConsole_RunsTheDaemonsOwnCLIAgainstTheRealDaemon() {
	env := s.NewEnv()
	c := env.Client(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	added, _ := s.exec(c, "arrow add "+ns)
	s.Equal("exit", added.Type)
	s.Equal(0, added.Code, added.Error)
	env.WaitForArrow(s.T(), ns, 30*time.Second)

	_, listing := s.exec(c, "arrow list")
	s.Contains(listing, "tool-a")
	s.NotContains(listing, "\x1b[", "the console renders plain text, never terminal escapes")

	installed, _ := s.exec(c, "install "+ns)
	s.Equal(0, installed.Code, installed.Error)
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	refused, _ := s.exec(c, "arrow remove "+ns)
	s.Equal(2, refused.Code)
	s.Contains(refused.Error, "--yes")
}

func (s *ConsoleSuite) TestConsole_RefusesWhatTheCLIWouldAllowButTheConsoleMustNot() {
	env := s.NewEnv()
	c := env.Client(s.T())

	for _, line := range []string{"daemon", "self-update /tmp/x", "context list", "auth devices list", "arrow seed x --file /etc/passwd", "list --server tcp://127.0.0.1:1"} {
		resp := c.ConsoleExec(line)
		resp.Body.Close()
		s.Equal(http.StatusForbidden, resp.StatusCode, line)
	}
}

func (s *ConsoleSuite) TestConsole_StreamsTheDaemonsLogsIncludingItsOwnAuditRecord() {
	previous := slog.Default()
	slog.SetDefault(slog.New(logring.Default().Tee(slog.NewTextHandler(io.Discard, nil))))
	defer slog.SetDefault(previous)
	env := s.NewEnv()
	c := env.Client(s.T())

	conn, err := c.DialConsoleLogs("?level=info")
	s.Require().NoError(err)
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	for {
		var f map[string]any
		s.Require().NoError(conn.ReadJSON(&f))
		if f["type"] == "ready" {
			break
		}
	}

	s.exec(c, "ps")

	for {
		var f map[string]any
		s.Require().NoError(conn.ReadJSON(&f))
		if f["msg"] == "exec" {
			s.Equal("console", f["component"])
			s.Equal("local", f["fields"].(map[string]any)["device"])
			return
		}
	}
}
