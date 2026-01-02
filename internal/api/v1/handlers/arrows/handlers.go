package arrows

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/apilibs"
	arrowusecases "github.com/rabbytesoftware/quiver/internal/api/v1/usecases/arrows"
)

type ArrowHandler struct {
	usecases *arrowusecases.ApiArrowUsecases
	apilib   *apilibs.ApiLib
}

func NewArrowHandler(uc *arrowusecases.ApiArrowUsecases) *ArrowHandler {

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

	arrow, warns, err := ah.usecases.Add(namespace, namespaceType, c.ClientIP())

	if err != nil {
		// Handle error
	}

	if len(warns) > 0 {
		// TODO: Return success message (206 PARTIAL CONTENT)
		fmt.Println("Arrow", arrow, "successfully added with this warnings:", warns)
	}

	// TODO: Return success message (201 CREATED)
	fmt.Println(arrow)
} // /

func (ah *ArrowHandler) RemoveArrow(c *gin.Context) {
	namespace := c.Param("namespace")

	if isNamespace := ah.apilib.IsNamespace(namespace, ""); isNamespace == false {
		// ERROR
	}

	warns, err := ah.usecases.Remove(namespace, c.ClientIP())

	if err != nil {
		// ERROR
	}

	if len(warns) > 0 {
		fmt.Println("Arrow", "successfully removed with this warnings:", warns)
	}

	// TODO: Return success message (200 OK)
	fmt.Println("Arrow successfully removed")

} // /

func (ah *ArrowHandler) ListenChannel(c *gin.Context) {}

type ExecuteMethodRequestDTO struct {
	Variables map[string]string `json:"variables"`
}

func (ah *ArrowHandler) ExecuteMethod(c *gin.Context) {
	var body ExecuteMethodRequestDTO
	namespace := c.Param("namespace")
	method := c.Param("method")

	if isNamespace := ah.apilib.IsNamespace(namespace, ""); isNamespace == false {
		// ERROR
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		// ERROR
	}

	variables := body.Variables
	if variables == nil {
		variables = map[string]string{}
	}

	warns, err := ah.usecases.ExecuteMethod(namespace, c.ClientIP(), method, variables)

	if err != nil {
		// ERROR
	}

	if len(warns) > 0 {
		// SUCCESS WITH WARNINGS
	}

	// SUCCESS

} // /

func (ah *ArrowHandler) ListArrows(c *gin.Context) {
	arrows, warns, err := ah.usecases.List()

	if err != nil {
		// ERROR
	}

	if len(warns) > 0 {
		// SUCCESS WITH WARNINGS
	}

	// TODO: Return arrows (200 OK)
	fmt.Println(arrows)
} // /

func (ah *ArrowHandler) GetArrow(c *gin.Context) {
	namespace := c.Param("namespace")

	if isNamespace := ah.apilib.IsNamespace(namespace, ""); isNamespace == false {
		// ERROR
	}

	arrow, warns, err := ah.usecases.Get(namespace)

	if err != nil {
		// ERROR
	}

	if len(warns) > 0 {
		// SUCCESS WITH WARNINGS
	}

	// TODO: Return arrow (200 OK)
	fmt.Println(arrow)
} // /

func (ah *ArrowHandler) StopMethod(c *gin.Context) {
	namespace := c.Param("namespace")
	method := c.Param("method")

	if isNamespace := ah.apilib.IsNamespace(namespace, ""); isNamespace == false {
		// ERROR
	}

	warns, err := ah.usecases.StopMethod(namespace, method)

	if err != nil {
		// ERROR
	}

	if len(warns) > 0 {
		// SUCCESS WITH WARNINGS
	}

	// TODO: Return success message (200 OK)
	fmt.Println("Method successfully stopped")
} // /

func (ah *ArrowHandler) KillMethod(c *gin.Context) {
	namespace := c.Param("namespace")
	method := c.Param("method")

	if isNamespace := ah.apilib.IsNamespace(namespace, ""); isNamespace == false {
		// ERROR
	}

	warns, err := ah.usecases.KillMethod(namespace, method)

	if err != nil {
		// ERROR
	}

	if len(warns) > 0 {
		// SUCCESS WITH WARNINGS
	}

	// TODO: Return success message (200 OK)
	fmt.Println("Method successfully killed")
} // /-
