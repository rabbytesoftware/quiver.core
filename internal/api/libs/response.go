package libs

import (
	"log/slog"
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

func WriteQueryWithStatus(c *gin.Context, status int, data any) {
	c.JSON(status, apiResponse{Success: status < 400, Data: data})
}

// WriteErr writes an error response.
//
// cause is the error the status and message were derived from. It is optional
// because many call sites reject a request outright and have no error value.
// When one is supplied and the status is 5xx, it is logged: a 5xx means the
// error could not be classified, so its wrapped chain is being replaced by a
// constant here and would otherwise leave no record anywhere of what actually
// failed. Classified 4xx responses already tell the caller what went wrong and
// are not logged.
func WriteErr(
	c *gin.Context,
	status int,
	message string,
	namespace string,
	cause ...error,
) {
	if status >= http.StatusInternalServerError && len(cause) > 0 && cause[0] != nil {
		slog.ErrorContext(
			c.Request.Context(), "api: unclassified error",
			"status", status,
			"namespace", namespace,
			"err", cause[0],
		)
	}

	c.JSON(status, apiResponse{
		Success:   false,
		Error:     &message,
		Namespace: namespace,
	})
}
