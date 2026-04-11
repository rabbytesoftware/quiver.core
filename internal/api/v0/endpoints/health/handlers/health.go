package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func Check(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
