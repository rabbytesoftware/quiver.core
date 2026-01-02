package apilibs

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	errors "github.com/rabbytesoftware/quiver/internal/core/errs"
)

type ApiResponse struct {
	c     *gin.Context
	start time.Time
}

type Payload interface {
	isPayload()
}

type PayloadBody[T any] struct {
	Data T `json:"data,omitempty"`
}

func (PayloadBody[T]) isPayload() {}

type Response struct {
	Success      bool            `json:"success"`
	Payload      Payload         `json:"payload,omitempty"`
	Error        *errors.Error   `json:"error,omitempty"`
	Warnings     []*errors.Error `json:"warnings,omitempty"`
	Timestamp    time.Time       `json:"timestamp"`
	ResponseTime string          `json:"responseTime"`
}

type ResponseInput struct {
	Status   int             `json:"status,omitempty"`
	Payload  Payload         `json:"payload,omitempty"`
	Error    *errors.Error   `json:"error,omitempty"`
	Warnings []*errors.Error `json:"warnings,omitempty"`
}

func NewApiResponse(c *gin.Context) *ApiResponse {
	return &ApiResponse{
		c:     c,
		start: time.Now(),
	}
}

func (ar *ApiResponse) ToResponse(in ResponseInput) {
	var statusCode int

	switch {
		case in.Status != 0 && in.Error == nil && len(in.Warnings) == 0:
			statusCode = in.Status
		case in.Error != nil:
			statusCode = int(in.Error.Code)
		case len(in.Warnings) > 0:
			statusCode = http.StatusPartialContent
		default:
			statusCode = http.StatusOK
	}

	success := statusCode >= 200 && statusCode < 300 && len(in.Warnings) == 0

	ar.c.JSON(statusCode, Response{
		Success:      success,
		Payload:      in.Payload,
		Error:        in.Error,
		Warnings:     in.Warnings,
		Timestamp:    time.Now().UTC(),
		ResponseTime: time.Since(ar.start).String(),
	})
}
