package middleware

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func RequestLogger() gin.HandlerFunc {
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

		args := []any{
			"method", method,
			"path", path,
			"status", statusCode,
			"latency", latency,
			"client_ip", clientIP,
			"body_size", bodySize,
			"type", "http_request",
		}

		msg := fmt.Sprintf("%s %s %d %v %s %d", method, path, statusCode, latency, clientIP, bodySize)

		ctx := c.Request.Context()
		switch {
		case statusCode >= 500:
			slog.ErrorContext(ctx, msg, args...)
		case statusCode >= 400:
			slog.WarnContext(ctx, msg, args...)
		default:
			slog.InfoContext(ctx, msg, args...)
		}
	}
}

func RequestRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				msg := fmt.Sprintf("Panic recovered: %v", err)
				slog.ErrorContext(
					c.Request.Context(), msg,
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"client_ip", c.ClientIP(),
					"type", "panic_recovery",
					"error", err,
				)
				c.AbortWithStatus(500)
			}
		}()
		c.Next()
	}
}
