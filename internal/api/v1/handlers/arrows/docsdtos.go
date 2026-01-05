package arrows

import (
	"time"
)

type responseError struct {
	Code    int                    `json:"code" example:"500"`
	Message string                 `json:"message" example:"something went wrong"`
	Details map[string]interface{} `json:"details,omitempty"`
}

type ErrorResponseDocsDTO struct {
	Success      bool           `json:"success" example:"false"`
	Error        *responseError `json:"error"`
	Timestamp    time.Time      `json:"timestamp" example:"2026-01-05T15:04:05Z"`
	ResponseTime string         `json:"responseTime" example:"150ms"`
}

type WarningResponseDocsDTO[T any] struct {
	Success      bool      `json:"success" example:"true"`
	Payload      T         `json:"payload,omitempty"`
	Warnings     []string  `json:"warnings" example:"some variable is required"`
	Timestamp    time.Time `json:"timestamp" example:"2026-01-05T15:04:05Z"`
	ResponseTime string    `json:"responseTime"  example:"215ms"`
}

type SuccessResponseDocsDTO[T any] struct {
	Success      bool      `json:"success" example:"true"`
	Payload      T         `json:"payload,omitempty"`
	Warnings     []string  `json:"warnings" example:"[]" description:"Always an empty array on success"`
	Timestamp    time.Time `json:"timestamp" example:"2026-01-05T15:04:05Z"`
	ResponseTime string    `json:"responseTime"  example:"300ms"`
}
