package console

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

func contextWith(
	value any,
	set bool,
) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	if set {
		c.Set(middleware.DeviceContextKey, value)
	}
	return c
}

func TestDevice_DeviceID_UsesTheAuthenticatedDevice(t *testing.T) {
	assert.Equal(t, "dev-1", deviceID(contextWith(auth.Device{ID: "dev-1"}, true)))
}

func TestDevice_DeviceID_FallsBackToLocal(t *testing.T) {
	assert.Equal(t, localDevice, deviceID(contextWith(nil, false)))
	assert.Equal(t, localDevice, deviceID(contextWith("not a device", true)))
	assert.Equal(t, localDevice, deviceID(contextWith(auth.Device{}, true)))
}

func TestDevice_BearerOf_ReadsOnlyBearerCredentials(t *testing.T) {
	cases := map[string]string{
		"Bearer abc":   "abc",
		"Bearer  abc ": "abc",
		"Basic abc":    "",
		"":             "",
		"bearer abc":   "",
	}
	for header, want := range cases {
		c := contextWith(nil, false)
		c.Request.Header.Set("Authorization", header)
		assert.Equal(t, want, bearerOf(c), header)
	}
}
