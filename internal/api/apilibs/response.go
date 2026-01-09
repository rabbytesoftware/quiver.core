package apilibs

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	errors "github.com/rabbytesoftware/quiver/internal/core/errs"
)

type Response[T any] struct {
	Success      bool          `json:"success"`
	Payload      T             `json:"payload,omitempty"`
	Error        *errors.Error `json:"error,omitempty"`
	Warnings     []error       `json:"warnings,omitempty"`
	Timestamp    time.Time     `json:"timestamp"`
	ResponseTime string        `json:"responseTime"`
}

type ResponseInput[T any] struct {
	StatusSuccess int           `json:"status,omitempty"`
	Payload       T             `json:"payload,omitempty"`
	Error         *errors.Error `json:"error,omitempty"`
	Warnings      []error       `json:"warnings,omitempty"`
}

func getRequestStartTime(c *gin.Context) (time.Time, bool) {
	val, ok := c.Get("request_start_time")
	if !ok {
		return time.Time{}, false
	}

	start, ok := val.(time.Time)
	return start, ok
}

func ToResponse[T any]( c *gin.Context, in ResponseInput[T]) {
	var statusCode int
	startTime, _ := getRequestStartTime(c)

	switch {
	case in.StatusSuccess != 0 && in.Error == nil && len(in.Warnings) == 0:
		statusCode = in.StatusSuccess
	case in.Error != nil:
		statusCode = int(in.Error.Code)
	case len(in.Warnings) > 0:
		statusCode = http.StatusPartialContent
	default:
		statusCode = http.StatusOK
	}

	success := statusCode >= 200 && statusCode < 300 && len(in.Warnings) == 0

	c.JSON(statusCode, Response[T]{
		Success:      success,
		Payload:      in.Payload,
		Error:        in.Error,
		Warnings:     in.Warnings,
		Timestamp:    time.Now().UTC(),
		ResponseTime: time.Since(startTime).String(),
	})
}
