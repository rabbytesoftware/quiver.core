package console_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	consoleendpoint "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/console"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

func TestRegister_Routes_MountsTheThreeConsoleRoutes(
	t *testing.T,
) {
	r := gin.New()

	consoleendpoint.Register(r.Group("/v0"), logring.New(), "v")

	var routes []string
	for _, route := range r.Routes() {
		routes = append(routes, route.Method+" "+route.Path)
	}
	assert.ElementsMatch(t, []string{"GET /v0/console/logs", "GET /v0/console/commands", "POST /v0/console/exec"}, routes)
}
