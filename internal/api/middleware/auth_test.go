package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

type stubAuthenticator struct {
	fn func(ctx context.Context, rawToken string) (auth.Device, error)
}

func (s stubAuthenticator) Authenticate(ctx context.Context, rawToken string) (auth.Device, error) {
	return s.fn(ctx, rawToken)
}

func newAuthTestRouter(gate *AuthGate, use gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", use, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func TestAuthGate_RequireBearer_NotRequired_PassesThrough(t *testing.T) {
	gate := NewAuthGate(stubAuthenticator{fn: func(context.Context, string) (auth.Device, error) {
		t.Fatal("Authenticate must not be called when the gate is not required")
		return auth.Device{}, nil
	}})

	r := newAuthTestRouter(gate, gate.RequireBearer())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthGate_RequireBearer_Required_NoHeader_Returns401(t *testing.T) {
	gate := NewAuthGate(stubAuthenticator{fn: func(context.Context, string) (auth.Device, error) {
		return auth.Device{}, nil
	}})
	gate.SetRequired(true)

	r := newAuthTestRouter(gate, gate.RequireBearer())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthGate_RequireBearer_Required_MalformedHeader_Returns401(t *testing.T) {
	gate := NewAuthGate(stubAuthenticator{fn: func(context.Context, string) (auth.Device, error) {
		return auth.Device{}, nil
	}})
	gate.SetRequired(true)

	r := newAuthTestRouter(gate, gate.RequireBearer())

	testCases := []string{"Bearer", "Basic abc123", "Bearer "}
	for _, header := range testCases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", header)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "header %q", header)
	}
}

func TestAuthGate_RequireBearer_Required_ValidToken_Succeeds(t *testing.T) {
	var gotToken string
	gate := NewAuthGate(stubAuthenticator{fn: func(_ context.Context, token string) (auth.Device, error) {
		gotToken = token
		return auth.Device{ID: "dev-1"}, nil
	}})
	gate.SetRequired(true)

	r := newAuthTestRouter(gate, gate.RequireBearer())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer abc123")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "abc123", gotToken)
}

func TestAuthGate_RequireBearer_Required_AuthenticateFails_MapsError(t *testing.T) {
	gate := NewAuthGate(stubAuthenticator{fn: func(context.Context, string) (auth.Device, error) {
		return auth.Device{}, apperrors.ErrUnauthorized
	}})
	gate.SetRequired(true)

	r := newAuthTestRouter(gate, gate.RequireBearer())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthGate_RequireBearer_Required_UnmappedError_Returns500(t *testing.T) {
	gate := NewAuthGate(stubAuthenticator{fn: func(context.Context, string) (auth.Device, error) {
		return auth.Device{}, errors.New("boom")
	}})
	gate.SetRequired(true)

	r := newAuthTestRouter(gate, gate.RequireBearer())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer whatever")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAuthGate_RequireLoopback_NotRequired_PassesThrough(t *testing.T) {
	gate := NewAuthGate(stubAuthenticator{})

	r := newAuthTestRouter(gate, gate.RequireLoopback())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.RemoteAddr = "203.0.113.5:12345"
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthGate_RequireLoopback_Required_NonLoopback_Returns403(t *testing.T) {
	gate := NewAuthGate(stubAuthenticator{})
	gate.SetRequired(true)

	r := newAuthTestRouter(gate, gate.RequireLoopback())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.RemoteAddr = "203.0.113.5:12345"
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAuthGate_RequireLoopback_Required_Loopback_Succeeds(t *testing.T) {
	gate := NewAuthGate(stubAuthenticator{})
	gate.SetRequired(true)

	r := newAuthTestRouter(gate, gate.RequireLoopback())

	testCases := []string{"127.0.0.1:54321", "[::1]:54321"}
	for _, addr := range testCases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.RemoteAddr = addr
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "addr %q", addr)
	}
}

func TestIsLoopback(t *testing.T) {
	testCases := []struct {
		name string
		addr string
		want bool
	}{
		{"loopback with port", "127.0.0.1:1234", true},
		{"ipv6 loopback with port", "[::1]:1234", true},
		{"loopback without port", "127.0.0.1", true},
		{"remote address", "203.0.113.5:1234", false},
		{"unparseable", "not-an-address", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isLoopback(tc.addr))
		})
	}
}

func TestBearerToken(t *testing.T) {
	testCases := []struct {
		name      string
		header    string
		wantToken string
		wantOK    bool
	}{
		{"valid", "Bearer abc123", "abc123", true},
		{"missing prefix", "abc123", "", false},
		{"empty token", "Bearer ", "", false},
		{"empty header", "", "", false},
		{"wrong scheme", "Basic abc123", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			token, ok := bearerToken(tc.header)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantToken, token)
		})
	}
}

func TestNewAuthGate_DefaultsToNotRequired(t *testing.T) {
	gate := NewAuthGate(stubAuthenticator{})
	require.False(t, gate.required.Load())
}

func TestAuthGate_Required_ReflectsSetRequired(t *testing.T) {
	gate := NewAuthGate(stubAuthenticator{})
	assert.False(t, gate.Required())

	gate.SetRequired(true)
	assert.True(t, gate.Required())

	gate.SetRequired(false)
	assert.False(t, gate.Required())
}
