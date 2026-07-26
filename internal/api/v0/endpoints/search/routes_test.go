package search

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestRegister_MountsSearchRoute(t *testing.T) {
	svc := &mocks.SearchService{}
	r := gin.New()
	Register(r.Group(""), svc)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/search?q=redis", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, svc.SearchCalls)
}
