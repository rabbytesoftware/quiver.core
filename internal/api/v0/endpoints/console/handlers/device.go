package console

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

const localDevice = "local"

func deviceID(
	c *gin.Context,
) string {
	value, ok := c.Get(middleware.DeviceContextKey)
	if !ok {
		return localDevice
	}

	device, ok := value.(auth.Device)
	if !ok || device.ID == "" {
		return localDevice
	}
	return device.ID
}

func bearerOf(
	c *gin.Context,
) string {
	token, found := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !found {
		return ""
	}
	return strings.TrimSpace(token)
}
