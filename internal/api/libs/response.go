package libs

import "github.com/gin-gonic/gin"

type apiResponse struct {
	Success   bool    `json:"success"`
	Error     *string `json:"error"`
	Namespace string  `json:"namespace,omitempty"`
	Data      any     `json:"data,omitempty"`
}

// WriteMutationOK writes a 201/200 success response with a namespace (no data body).
func WriteMutationOK(
	c *gin.Context,
	status int,
	namespace string,
) {
	c.JSON(status, apiResponse{Success: true, Namespace: namespace})
}

// WriteQueryOK writes a 200 success response with a data payload.
func WriteQueryOK(c *gin.Context, data any) {
	c.JSON(200, apiResponse{Success: true, Data: data})
}

// WriteErr writes an error response with the given HTTP status, message, and optional namespace.
func WriteErr(
	c *gin.Context,
	status int,
	message string,
	namespace string,
) {
	c.JSON(status, apiResponse{
		Success:   false,
		Error:     &message,
		Namespace: namespace,
	})
}
