package home

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

func TestRegister_MountsBothRoutes(t *testing.T) {
	svc := &mocks.HomeService{}
	r := gin.New()
	Register(r.Group(""), svc)

	read := httptest.NewRecorder()
	r.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/home", nil))
	refresh := httptest.NewRecorder()
	r.ServeHTTP(refresh, httptest.NewRequest(http.MethodPost, "/home/refresh", nil))

	assert.Equal(t, http.StatusOK, read.Code)
	assert.Equal(t, http.StatusAccepted, refresh.Code)
	assert.Equal(t, 1, svc.HomeCalls)
	assert.Equal(t, 1, svc.RefreshCalls)
}
