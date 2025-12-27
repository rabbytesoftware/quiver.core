package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/core/watcher"
	"github.com/sirupsen/logrus"
)

func WatcherLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)

		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()
		bodySize := c.Writer.Size()

		if raw != "" {
			path = path + "?" + raw
		}

		message := fmt.Sprintf("%s %s %d %v %s %d",
			method,
			path,
			statusCode,
			latency,
			clientIP,
			bodySize,
		)

		logEntry := watcher.WithFields(logrus.Fields{
			"method":    method,
			"path":      path,
			"status":    statusCode,
			"latency":   latency,
			"client_ip": clientIP,
			"body_size": bodySize,
			"type":      "http_request",
		})

		switch {
		case statusCode >= 500:
			logEntry.Error(message)
		case statusCode >= 400:
			logEntry.Warn(message)
		default:
			logEntry.Info(message)
		}
	}
}

func WatcherRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Log the panic with Watcher
				message := fmt.Sprintf("Panic recovered: %v", err)

				// Log with structured fields for file output
				watcher.WithFields(logrus.Fields{
					"method":    c.Request.Method,
					"path":      c.Request.URL.Path,
					"client_ip": c.ClientIP(),
					"type":      "panic_recovery",
					"error":     err,
				}).Error(message)

				// Also send to UI via Watcher's direct method
				watcher.Error("%s", message)

				// Return 500 error
				c.AbortWithStatus(500)
			}
		}()
		c.Next()
	}
}
