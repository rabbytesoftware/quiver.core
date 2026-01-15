package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
)

const requestStartTimeKey = "request_start_time"

func RequestTimer() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Set(requestStartTimeKey, start)

		c.Next()
	}
}
