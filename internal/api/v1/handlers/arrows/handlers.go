package arrows

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/apilibs"
	arrowusecases "github.com/rabbytesoftware/quiver/internal/api/v1/usecases/arrows"
)

type ArrowHandler struct {
	uc     *arrowusecases.ApiArrowUsescases
	apilib *apilibs.ApiLib
}

func NewArrowHandler(uc *arrowusecases.ApiArrowUsescases) *ArrowHandler {

	return &ArrowHandler{
		uc:     uc,
		apilib: apilibs.NewApiLib(),
	}
}

func (ah *ArrowHandler) verifyNamespaceSchema(ns string) (string, error) {
	if isDirectory := ah.apilib.IsDirectory(ns); isDirectory == true {
		return "directory", nil
	}

	if isUrl := ah.apilib.IsUrl(ns); isUrl == true {
		return "url", nil
	}

	if isNamespace := ah.apilib.IsNamespace(ns, ""); isNamespace == true {
		return "namespace", nil
	}

	// TODO: Better error message
	return "", fmt.Errorf("invalid schema")
}

func (ah *ArrowHandler) AddArrow(c *gin.Context) {
	namespace := c.Param("namespace")

	ns, err := ah.verifyNamespaceSchema(namespace)

	switch ns {
	case "directory":
		// HandleDirectory
	case "url":
		// HandlerURL
		return
	case "namespace":
		// HandleNamespace
		return
	default:

		c.JSON(http.StatusBadRequest, gin.H{
			"error": err,
		})
	}
}

func (ah *ArrowHandler) RemoveArrow(c *gin.Context) {}

func (ah *ArrowHandler) ListenChannel(c *gin.Context) {}

func (ah *ArrowHandler) ExecuteMethod(c *gin.Context) {}

func (ah *ArrowHandler) ListArrows(c *gin.Context) {}

func (ah *ArrowHandler) GetArrow(c *gin.Context) {}
