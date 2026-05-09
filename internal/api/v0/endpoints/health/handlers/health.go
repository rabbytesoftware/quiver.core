package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Check returns the health status of the daemon.
//
// @Summary      Health check
// @Description  Returns {"status":"ok"} when the daemon is running.
// @Tags         system
// @Produce      json
// @Success      200  {object}  map[string]string  "Daemon is healthy"
// @Router       /health [get]
func Check(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
