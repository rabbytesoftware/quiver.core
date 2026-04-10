package libs

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type apiResponse struct {
	Success   bool    `json:"success"`
	Error     *string `json:"error"`
	Namespace string  `json:"namespace,omitempty"`
	Data      any     `json:"data,omitempty"`
}

func WriteMutationOK(
	c *gin.Context,
	status int,
	namespace string,
) {
	c.JSON(status, apiResponse{Success: true, Namespace: namespace})
}

func WriteQueryOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, apiResponse{Success: true, Data: data})
}

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
