package auth

import (
	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	authhandlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/auth/handlers"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

// Register mounts the device-pairing endpoints. Admin operations (generate,
// list, revoke) are loopback-gated rather than bearer-gated: they are how a
// device becomes trusted in the first place, so they cannot themselves
// require an existing token. Redeem carries no auth at all — the pairing code
// is the credential — and is instead rate-limited against brute-force search
// across the code space.
func Register(
	rg *gin.RouterGroup,
	svc usecases.AuthUsecase,
	gate *middleware.AuthGate,
	limiter *middleware.RateLimiter,
) {
	h := authhandlers.New(svc)

	rg.POST("/auth/pairing", gate.RequireLoopback(), h.GeneratePairingCode)
	rg.POST("/auth/pairing/redeem", limiter.Middleware(), h.Redeem)
	rg.GET("/auth/devices", gate.RequireLoopback(), h.ListDevices)
	rg.DELETE("/auth/devices/:id", gate.RequireLoopback(), h.RevokeDevice)
}
