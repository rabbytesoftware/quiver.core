package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	"github.com/rabbytesoftware/quiver.core/internal/api/libs/apierr"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// DeviceContextKey is where RequireBearer stores the authenticated device on
// a successful request.
const DeviceContextKey = "auth.device"

// Authenticator resolves a bearer token to its device. Satisfied by
// usecases.AuthUsecase; declared locally so this package does not import the
// app layer.
type Authenticator interface {
	Authenticate(ctx context.Context, rawToken string) (auth.Device, error)
}

// AuthGate decides whether device-pairing auth applies to this daemon
// instance. It is required only when the daemon is reachable over tcp:// — a
// unix:// connection is already trusted by filesystem permissions, so the
// zero value (not required) makes every gated handler a no-op.
//
// Every gated handler is built into the Gin router once, at construction
// time, before the daemon's listener scheme is known (internal.Start resolves
// it after the router is already built). SetRequired mutates the same *AuthGate
// those closures already captured, so it takes effect for requests served
// after the call without rebuilding any route. It is set exactly once, by
// internal.Container.Start, before the listener starts accepting connections,
// then only ever read concurrently by request goroutines — hence atomic.Bool
// rather than a plain field.
type AuthGate struct {
	required atomic.Bool
	svc      Authenticator
}

// NewAuthGate constructs a gate backed by svc. Required starts false (unix://
// behaviour) until SetRequired says otherwise.
func NewAuthGate(
	svc Authenticator,
) *AuthGate {
	return &AuthGate{svc: svc}
}

// SetRequired flips whether the gated handlers enforce anything.
func (g *AuthGate) SetRequired(
	required bool,
) {
	g.required.Store(required)
}

// RequireBearer rejects a request with no valid Authorization: Bearer token,
// unless the gate is not required.
func (g *AuthGate) RequireBearer() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !g.required.Load() {
			c.Next()
			return
		}

		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			libs.WriteErr(c, http.StatusUnauthorized, "missing or malformed bearer token", "")
			c.Abort()
			return
		}

		device, err := g.svc.Authenticate(c.Request.Context(), token)
		if err != nil {
			status, msg := apierr.StatusAndMessage(err)
			libs.WriteErr(c, status, msg, "")
			c.Abort()
			return
		}

		c.Set(DeviceContextKey, device)
		c.Next()
	}
}

// RequireLoopback rejects a request whose remote address is not the daemon's
// own host, unless the gate is not required. Used for the pairing admin
// endpoints (generate code, list/revoke devices), which must never be
// reachable by anything but the CLI running on the same machine as the
// daemon — see the design note on why the redeem endpoint's rate limiter
// cannot substitute for this.
func (g *AuthGate) RequireLoopback() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !g.required.Load() {
			c.Next()
			return
		}

		if !isLoopback(c.Request.RemoteAddr) {
			libs.WriteErr(c, http.StatusForbidden, "reachable only from the daemon's own host", "")
			c.Abort()
			return
		}

		c.Next()
	}
}

func bearerToken(
	header string,
) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}

	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", false
	}

	return token, true
}

func isLoopback(
	remoteAddr string,
) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}

	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
