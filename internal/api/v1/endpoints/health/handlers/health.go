package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Check handles GET /v1/health. Returns 200 {"status":"ok"}.
func Check(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
