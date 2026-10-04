package console_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	consoleendpoint "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/console"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

func TestRegister_MountsTheThreeConsoleRoutes(t *testing.T) {
	r := gin.New()

	consoleendpoint.Register(r.Group("/v0"), logring.New(), "v")

	assert.Len(t, r.Routes(), 3)
}
