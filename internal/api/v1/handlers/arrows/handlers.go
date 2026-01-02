package arrows

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/apilibs"
	arrowusecases "github.com/rabbytesoftware/quiver/internal/api/v1/usecases/arrows"
)

type ArrowHandler struct {
	usecases *arrowusecases.ApiArrowUsescases
	apilib   *apilibs.ApiLib
}

func NewArrowHandler(uc *arrowusecases.ApiArrowUsescases) *ArrowHandler {

	return &ArrowHandler{
		usecases: uc,
		apilib:   apilibs.NewApiLib(),
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

	namespaceType, verificationErr := ah.verifyNamespaceSchema(namespace)

	if verificationErr != nil {
		// RETURN ERROR
	}

	arrow, errs, err := ah.usecases.Add(namespace, namespaceType, c.ClientIP())

	if err != nil {
		// Handle error
	}

	if len(errs) > 0 {
		// Handle errors
	}

	fmt.Println(arrow)
}

func (ah *ArrowHandler) RemoveArrow(c *gin.Context) {}

func (ah *ArrowHandler) ListenChannel(c *gin.Context) {}

func (ah *ArrowHandler) ExecuteMethod(c *gin.Context) {}

func (ah *ArrowHandler) ListArrows(c *gin.Context) {}

func (ah *ArrowHandler) GetArrow(c *gin.Context) {}
