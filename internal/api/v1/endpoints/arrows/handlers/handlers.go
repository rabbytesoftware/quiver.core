package arrows

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	apilibs "github.com/rabbytesoftware/quiver/internal/api/libs"
	errors "github.com/rabbytesoftware/quiver/internal/core/errs"
	arrowusecases "github.com/rabbytesoftware/quiver/internal/usecases/arrows"
)

type ArrowHandler interface {
	AddArrow(c *gin.Context)
	RemoveArrow(c *gin.Context)
	ExecuteMethod(c *gin.Context)
	ListArrows(c *gin.Context)
	GetArrow(c *gin.Context)
	StopMethod(c *gin.Context)
	KillMethod(c *gin.Context)
	ListenChannel(c *gin.Context)
}

type ArrowHandlerImpl struct {
	usecases *arrowusecases.ApiArrowUsecases
}

func NewArrowHandler(uc *arrowusecases.ApiArrowUsecases) ArrowHandler {

	return &ArrowHandlerImpl{
		usecases: uc,
	}
}

func verifyNamespaceSchema(ns string) (string, error) {

	if apilibs.IsDirectory(ns) {
		return "directory", nil
	}

	if apilibs.IsUrl(ns) {
		return "url", nil
	}

	if apilibs.IsNamespace(ns, "") {
		return "namespace", nil
	}

	return "", fmt.Errorf(
		"invalid namespace schema: %q does not match directory, url, or namespace format",
		ns,
	)
}

func decodeUrl(val string) (string, error) {
	raw := strings.TrimPrefix(val, "/")

	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return "", fmt.Errorf("invalid path encoding")
	}

	if strings.Contains(decoded, "..") {
		return "", fmt.Errorf("invalid path")
	}

	decoded = strings.TrimSuffix(decoded, "/")

	return decoded, nil
}

func (ah *ArrowHandlerImpl) AddArrow(c *gin.Context) {
	val := c.Param("value")
	force := c.Query("force") == "true"

	decodedVal, decodeErr := decodeUrl(val)

	if decodeErr != nil {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Message: fmt.Sprintf("failed while decoding: %s", decodeErr.Error()),
				Code:    errors.InvalidRequestCode,
			},
		))
	}

	namespaceType, verificationErr := verifyNamespaceSchema(decodedVal)

	if verificationErr != nil {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Message: fmt.Sprintf("invalid namespace schema: %s", verificationErr.Error()),
				Code:    errors.InvalidRequestCode,
			},
		))
	}

	arrow, warns, err := ah.usecases.Add(val, namespaceType, c.ClientIP(), force)

	if err != nil {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Code:    errors.InvalidRequestCode,
				Message: fmt.Sprintf("could not add arrow: %s", err.Error()),
			},
		))
		return
	}

	apilibs.ToResponse(c, apilibs.WithSuccessCode(int(errors.CreatedCode)), apilibs.WithWarnings(warns), apilibs.WithPayload(arrow))
}

func (ah *ArrowHandlerImpl) RemoveArrow(c *gin.Context) {
	namespace := c.Param("namespace")

	if !apilibs.IsNamespace(namespace, "") {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Code:    errors.InvalidRequestCode,
				Message: "invalid namespace",
			},
		))
		return
	}

	warns, err := ah.usecases.Remove(namespace, c.ClientIP())
	if err != nil {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Code:    errors.InternalServerCode,
				Message: err.Error(),
			},
		))
		return
	}

	apilibs.ToResponse(c, apilibs.WithSuccessCode(int(errors.NoContentCode)), apilibs.WithWarnings(warns))
}

func (ah *ArrowHandlerImpl) ExecuteMethod(c *gin.Context) {

	var body ExecuteMethodRequestDTO
	namespace := c.Param("namespace")
	method := c.Param("method")

	if !apilibs.IsNamespace(namespace, "") {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Code:    errors.InvalidRequestCode,
				Message: "invalid namespace",
			},
		))
		return
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Code:    errors.InvalidRequestCode,
				Message: err.Error(),
			},
		))
		return
	}

	variables := body.Variables
	if variables == nil {
		variables = map[string]string{}
	}

	warns, err := ah.usecases.ExecuteMethod(namespace, c.ClientIP(), method, variables)
	if err != nil {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Code:    errors.InternalServerCode,
				Message: err.Error(),
			},
		))
		return
	}

	apilibs.ToResponse(c, apilibs.WithSuccessCode(int(errors.AcceptedCode)), apilibs.WithWarnings(warns))
}

func (ah *ArrowHandlerImpl) ListArrows(c *gin.Context) {
	arrows, warns, err := ah.usecases.List()
	if err != nil {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Code:    errors.InternalServerCode,
				Message: err.Error(),
			},
		))
		return
	}

	apilibs.ToResponse(c, apilibs.WithSuccessCode(int(errors.SuccessCode)), apilibs.WithPayload(arrows), apilibs.WithWarnings(warns))
}

func (ah *ArrowHandlerImpl) GetArrow(c *gin.Context) {
	namespace := c.Param("namespace")

	if !apilibs.IsNamespace(namespace, "") {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Code:    errors.InvalidRequestCode,
				Message: "invalid namespace",
			},
		))
		return
	}

	arrow, warns, err := ah.usecases.Get(namespace)
	if err != nil {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Code:    errors.NotFoundCode,
				Message: err.Error(),
			},
		))
		return
	}

	apilibs.ToResponse(c, apilibs.WithSuccessCode(int(errors.SuccessCode)), apilibs.WithPayload(*arrow), apilibs.WithWarnings(warns))
}

func (ah *ArrowHandlerImpl) StopMethod(c *gin.Context) {
	namespace := c.Param("namespace")
	method := c.Param("method")

	if !apilibs.IsNamespace(namespace, "") {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Code:    errors.InvalidRequestCode,
				Message: "invalid namespace",
			},
		))
		return
	}

	warns, err := ah.usecases.StopMethod(namespace, method)
	if err != nil {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Code:    errors.InternalServerCode,
				Message: err.Error(),
			},
		))
		return
	}

	apilibs.ToResponse(c, apilibs.WithSuccessCode(int(errors.SuccessCode)), apilibs.WithWarnings(warns))
}

func (ah *ArrowHandlerImpl) KillMethod(c *gin.Context) {
	namespace := c.Param("namespace")
	method := c.Param("method")

	if !apilibs.IsNamespace(namespace, "") {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Code:    errors.InvalidRequestCode,
				Message: "invalid namespace",
			},
		))
		return
	}

	warns, err := ah.usecases.KillMethod(namespace, method)
	if err != nil {
		apilibs.ToResponse(c, apilibs.WithError(
			&errors.Error{
				Code:    errors.InternalServerCode,
				Message: err.Error(),
			},
		))
		return
	}

	apilibs.ToResponse(c, apilibs.WithSuccessCode(int(errors.SuccessCode)), apilibs.WithWarnings(warns))
}

// TODO: Implement method
func (ah *ArrowHandlerImpl) ListenChannel(c *gin.Context) {
}
