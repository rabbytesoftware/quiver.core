package ui_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/ui"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func TestRegister_MountsUIUnderPrefix(t *testing.T) {
	svc := &mocks.SurfaceService{HandlerResult: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})}
	r := gin.New()
	ui.Register(r.Group("/v0"), svc)

	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodHead} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(m, "/v0/ui/a%2Fb/x", nil))
		require.Equal(t, http.StatusTeapot, rec.Code, m)
	}
}
