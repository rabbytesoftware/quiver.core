package console_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/console"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/console/command"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

type stubAuthenticator struct{}

func (stubAuthenticator) Authenticate(
	_ context.Context,
	token string,
) (auth.Device, error) {
	if token != "good" {
		return auth.Device{}, apperrors.ErrUnauthorized
	}
	return auth.Device{ID: "dev-1", State: auth.DeviceStateActive}, nil
}

type daemonLog struct {
	mu       sync.Mutex
	requests []string
	auth     []string
}

func (d *daemonLog) snapshot() ([]string, []string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return append([]string(nil), d.requests...), append([]string(nil), d.auth...)
}

type stack struct {
	console *httptest.Server
	daemon  *daemonLog
}

func newStack(
	t *testing.T,
	gateRequired bool,
) *stack {
	t.Helper()

	log := &daemonLog{}
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.mu.Lock()
		log.requests = append(log.requests, r.Method+" "+r.URL.Path)
		log.auth = append(log.auth, r.Header.Get("Authorization"))
		log.mu.Unlock()
		_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
	}))
	t.Cleanup(daemon.Close)

	gate := middleware.NewAuthGate(stubAuthenticator{})
	gate.SetRequired(gateRequired)
	r := gin.New()
	protected := r.Group("")
	protected.Use(gate.RequireBearer())
	exec := command.New(command.Options{ServerURI: daemon.URL})
	console.Register(protected, logring.New(10), exec)

	server := httptest.NewServer(r)
	t.Cleanup(server.Close)
	return &stack{console: server, daemon: log}
}

func (s *stack) exec(
	t *testing.T,
	line string,
	token string,
) (int, string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, s.console.URL+"/console/exec", strings.NewReader(`{"line":"`+line+`"}`))
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var body strings.Builder
	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		body.Write(buf[:n])
		if readErr != nil {
			break
		}
	}
	return resp.StatusCode, body.String()
}

func TestIntegration_TCPModeRequiresAValidBearerToken(t *testing.T) {
	s := newStack(t, true)

	missing, _ := s.exec(t, "arrow list", "")
	invalid, _ := s.exec(t, "arrow list", "bad")

	assert.Equal(t, http.StatusUnauthorized, missing)
	assert.Equal(t, http.StatusUnauthorized, invalid)
	requests, _ := s.daemon.snapshot()
	assert.Empty(t, requests, "an unauthenticated caller must never reach a command")
}

func TestIntegration_ACommandRunsWithTheCallersOwnCredentials(t *testing.T) {
	s := newStack(t, true)

	status, body := s.exec(t, "arrow list", "good")

	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"type":"exit","code":0`)
	requests, auths := s.daemon.snapshot()
	assert.Equal(t, []string{"GET /v0/arrow"}, requests)
	assert.Equal(t, []string{"Bearer good"}, auths)
}

func TestIntegration_UnixModeNeedsNoTokenAndForwardsNone(t *testing.T) {
	s := newStack(t, false)

	status, body := s.exec(t, "arrow list", "")

	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"type":"exit","code":0`)
	_, auths := s.daemon.snapshot()
	assert.Equal(t, []string{""}, auths)
}

func TestIntegration_AuthenticatedCallersStillCannotReachUnlistedCommands(t *testing.T) {
	s := newStack(t, true)

	for _, line := range []string{"daemon", "context list", "auth devices list", "arrow seed x", "self-update x"} {
		status, _ := s.exec(t, line, "good")
		assert.Equal(t, http.StatusForbidden, status, line)
	}
	requests, _ := s.daemon.snapshot()
	assert.Empty(t, requests)
}

func TestIntegration_TheLogStreamAndCommandListSitBehindTheSameGate(t *testing.T) {
	s := newStack(t, true)

	for _, path := range []string{"/console/logs", "/console/commands"} {
		resp, err := http.Get(s.console.URL + path)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, path)
	}
}
