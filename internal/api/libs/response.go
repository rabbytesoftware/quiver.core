package libs

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/libs/apierr"
)

type Response struct {
	Success      bool          `json:"success"`
	Payload      interface{}   `json:"payload,omitempty"`
	Error        *apierr.Error `json:"error,omitempty"`
	Warnings     []string      `json:"warnings,omitempty"`
	Timestamp    time.Time     `json:"timestamp"`
	ResponseTime string        `json:"responseTime"`
}

type responseOptions struct {
	Payload  interface{}   `json:"payload,omitempty"`
	Error    *apierr.Error `json:"error,omitempty"`
	Warnings []string      `json:"warnings,omitempty"`
	Code     int           `json:"code,omitempty"`
}

type ResponseOption func(*responseOptions)

func defaultResponseOptions() *responseOptions {
	return &responseOptions{
		Payload:  nil,
		Error:    nil,
		Warnings: nil,
		Code:     200,
	}
}

func WithError(err *apierr.Error) ResponseOption {
	return func(o *responseOptions) {
		o.Error = err
	}
}

func WithWarnings(warns []error) ResponseOption {
	return func(o *responseOptions) {
		o.Warnings = errorsToStrings(warns)
	}
}

func WithPayload(p interface{}) ResponseOption {
	return func(o *responseOptions) {
		o.Payload = p
	}
}

func WithSuccessCode(code int) ResponseOption {
	return func(o *responseOptions) {
		if code < 200 || code >= 300 {
			return
		}

		o.Code = code
	}
}

func getRequestStartTime(c *gin.Context) (time.Time, bool) {
	val, ok := c.Get("request_start_time")
	if !ok {
		return time.Time{}, false
	}

	start, ok := val.(time.Time)
	return start, ok
}

func errorsToStrings(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.Error())
	}
	return out
}

func ToResponse(c *gin.Context, opts ...ResponseOption) {
	var statusCode int
	startTime, _ := getRequestStartTime(c)
	o := defaultResponseOptions()

	for _, opt := range opts {
		opt(o)
	}

	switch {
	case o.Code != 0 && o.Error == nil && len(o.Warnings) == 0:
		statusCode = o.Code
	case o.Error != nil:
		statusCode = int(o.Error.Code)
	case len(o.Warnings) > 0:
		statusCode = http.StatusPartialContent
	default:
		statusCode = http.StatusOK
	}

	success := statusCode >= 200 && statusCode < 300 && len(o.Warnings) == 0

	c.JSON(statusCode, Response{
		Success:      success,
		Payload:      o.Payload,
		Error:        o.Error,
		Warnings:     o.Warnings,
		Timestamp:    time.Now().UTC(),
		ResponseTime: time.Since(startTime).String(),
	})
}
