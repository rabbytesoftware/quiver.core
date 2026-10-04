package console_test

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/console"
	"github.com/rabbytesoftware/quiver.core/internal/console/command"
	"github.com/rabbytesoftware/quiver.core/internal/console/logring"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestRegister_MountsTheConsoleRoutes(t *testing.T) {
	r := gin.New()
	console.Register(r.Group(""), logring.New(1), command.New(command.Options{}))

	mounted := make(map[string]bool)
	for _, route := range r.Routes() {
		mounted[route.Method+" "+route.Path] = true
	}

	assert.True(t, mounted["GET /console/logs"])
	assert.True(t, mounted["GET /console/commands"])
	assert.True(t, mounted["POST /console/exec"])
	assert.Len(t, mounted, 3)
}
