package arrows

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/apilibs"
	arrowusecases "github.com/rabbytesoftware/quiver/internal/api/v1/usecases/arrows"
	errors "github.com/rabbytesoftware/quiver/internal/core/errs"
	arrowmodel "github.com/rabbytesoftware/quiver/internal/models/arrow"
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

	if ah.apilib.IsDirectory(ns) {
		return "directory", nil
	}

	if ah.apilib.IsUrl(ns) {
		return "url", nil
	}

	if ah.apilib.IsNamespace(ns, "") {
		return "namespace", nil
	}

	return "", fmt.Errorf(
		"invalid namespace schema: %q does not match directory, url, or namespace format",
		ns,
	)
}

// AddArrow 		godoc
// @Summary 		Add an arrow
// @Description
// Creates a new arrow.
//
// - Returns **200 OK** when the arrow is created successfully with no warnings
// - Returns **206 Partial Content** when the arrow is created but warnings are present
// - Returns **400 Bad Request** when the request is invalid
//
// @Tags 				v1 / Arrows
// @Produce 		json
// @Param 			namespace path string true "Arrow namespace (namespace | url | directory)"
// @Success     200 {object} SuccessResponseDocsDTO[arrowmodel.Arrow] "Success without warnings"
// @Success     206 {object} WarningResponseDocsDTO[arrowmodel.Arrow] "Success with warnings"
// @Failure     400 {object} ErrorResponseDocsDTO "Invalid request"
// @Router 			/api/v1/arrow/{namespace} [post]
func (ah *ArrowHandler) AddArrow(c *gin.Context) {
	resp := apilibs.NewApiResponse(c)
	namespace := c.Param("namespace")

	namespaceType, verificationErr := ah.verifyNamespaceSchema(namespace)

	if verificationErr != nil {
		resp.ToResponse(apilibs.ResponseInput{
			Error: &errors.Error{
				Message: fmt.Sprintf("invalid namespace schema: %s", verificationErr.Error()),
				Code:    errors.InvalidRequestCode,
			},
		})
	}

	arrow, warns, err := ah.usecases.Add(namespace, namespaceType, c.ClientIP())

	if err != nil {
		resp.ToResponse(apilibs.ResponseInput{
			Error: &errors.Error{
				Code:    errors.InvalidRequestCode,
				Message: fmt.Sprintf("could not add arrow: %s", err.Error()),
			},
		})
		return
	}

	resp.ToResponse(apilibs.ResponseInput{
		StatusSuccess: int(errors.CreatedCode),
		Warnings:      warns,
		Payload: apilibs.PayloadBody[arrowmodel.Arrow]{
			Data: *arrow,
		},
	})
}

// RemoveArrow 		godoc
// @Summary 		Remove an arrow
// @Description
// Remove an arrow.
//
// - Returns **204 No Content** when the arrow is removed successfully with no warnings
// - Returns **206 Partial Content** when the arrow is removed but warnings are present
// - Returns **400 Bad Request** when the request is invalid
// - Returns **500 Internal Error** when something went wrong internally
//
// @Tags 				v1 / Arrows
// @Produce 		json
// @Param 			namespace path string true "Arrow namespace (namespace | url | directory)"
// @Success     204 {object} SuccessResponseWithoutPayloadDocsDTO "Success without warnings"
// @Success     206 {object} WarningResponseWithoutPayloadDocsDTO "Success with warnings"
// @Failure     400 {object} ErrorResponseDocsDTO "Invalid request"
// @Failure     500 {object} ErrorResponseDocsDTO "Internal error"
// @Router 			/api/v1/arrow/{namespace} [delete]
func (ah *ArrowHandler) RemoveArrow(c *gin.Context) {
	resp := apilibs.NewApiResponse(c)
	namespace := c.Param("namespace")

	if !ah.apilib.IsNamespace(namespace, "") {
		resp.ToResponse(apilibs.ResponseInput{
			Error: &errors.Error{
				Code:    errors.InvalidRequestCode,
				Message: "invalid namespace",
			},
		})
		return
	}

	warns, err := ah.usecases.Remove(namespace, c.ClientIP())
	if err != nil {
		resp.ToResponse(apilibs.ResponseInput{
			Error: &errors.Error{
				Code:    errors.InternalServerCode,
				Message: err.Error(),
			},
		})
		return
	}

	resp.ToResponse(apilibs.ResponseInput{
		StatusSuccess: int(errors.NoContentCode),
		Warnings:      warns,
	})
}

// RemoveArrow 		godoc
// @Summary 		Execute arrow method
// @Description
// Execute  arrow method.
//
// - Returns **202 Accepted** when the arrow method is executed successfully with no warnings
// - Returns **206 Partial Content** when the arrow method is executed but warnings are present
// - Returns **400 Bad Request** when the request is invalid
// - Returns **500 Internal Error** when something went wrong internally
//
// @Tags 				v1 / Arrows
// @Accept			json
// @Produce 		json
// @Param 			namespace path string true "Arrow namespace (namespace | url | directory)"
// @Param 			method path string true "Method name"
// @Param 			variables body ExecuteMethodRequestDTO true "Execute method variables"
// @Success     202 {object} SuccessResponseWithoutPayloadDocsDTO "Success without warnings"
// @Success     206 {object} WarningResponseWithoutPayloadDocsDTO "Success with warnings"
// @Failure     400 {object} ErrorResponseDocsDTO "Invalid request"
// @Failure     500 {object} ErrorResponseDocsDTO "Internal error"
// @Router 			/api/v1/arrow/{namespace}/{method} [POST]
func (ah *ArrowHandler) ExecuteMethod(c *gin.Context) {
	resp := apilibs.NewApiResponse(c)

	var body ExecuteMethodRequestDTO
	namespace := c.Param("namespace")
	method := c.Param("method")

	if !ah.apilib.IsNamespace(namespace, "") {
		resp.ToResponse(apilibs.ResponseInput{
			Error: &errors.Error{
				Code:    errors.InvalidRequestCode,
				Message: "invalid namespace",
			},
		})
		return
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		resp.ToResponse(apilibs.ResponseInput{
			Error: &errors.Error{
				Code:    errors.InvalidRequestCode,
				Message: err.Error(),
			},
		})
		return
	}

	variables := body.Variables
	if variables == nil {
		variables = map[string]string{}
	}

	warns, err := ah.usecases.ExecuteMethod(namespace, c.ClientIP(), method, variables)
	if err != nil {
		resp.ToResponse(apilibs.ResponseInput{
			Error: &errors.Error{
				Code:    errors.InternalServerCode,
				Message: err.Error(),
			},
		})
		return
	}

	resp.ToResponse(apilibs.ResponseInput{
		StatusSuccess: int(errors.AcceptedCode),
		Warnings:      warns,
	})
}

func (ah *ArrowHandler) ListArrows(c *gin.Context) {
	resp := apilibs.NewApiResponse(c)

	arrows, warns, err := ah.usecases.List()
	if err != nil {
		resp.ToResponse(apilibs.ResponseInput{
			Error: &errors.Error{
				Code:    errors.InternalServerCode,
				Message: err.Error(),
			},
			Warnings: warns,
		})
		return
	}

	resp.ToResponse(apilibs.ResponseInput{
		StatusSuccess: int(errors.SuccessCode),
		Payload: apilibs.PayloadBody[map[string]arrowmodel.Arrow]{
			Data: arrows,
		},
		Warnings: warns,
	})
}

func (ah *ArrowHandler) GetArrow(c *gin.Context) {
	resp := apilibs.NewApiResponse(c)
	namespace := c.Param("namespace")

	if !ah.apilib.IsNamespace(namespace, "") {
		resp.ToResponse(apilibs.ResponseInput{
			Error: &errors.Error{
				Code:    errors.InvalidRequestCode,
				Message: "invalid namespace",
			},
		})
		return
	}

	arrow, warns, err := ah.usecases.Get(namespace)
	if err != nil {
		resp.ToResponse(apilibs.ResponseInput{
			Error: &errors.Error{
				Code:    errors.NotFoundCode,
				Message: err.Error(),
			},
			Warnings: warns,
		})
		return
	}

	resp.ToResponse(apilibs.ResponseInput{
		StatusSuccess: int(errors.SuccessCode),
		Payload: apilibs.PayloadBody[arrowmodel.Arrow]{
			Data: *arrow,
		},
		Warnings: warns,
	})
}

func (ah *ArrowHandler) StopMethod(c *gin.Context) {
	resp := apilibs.NewApiResponse(c)
	namespace := c.Param("namespace")
	method := c.Param("method")

	if !ah.apilib.IsNamespace(namespace, "") {
		resp.ToResponse(apilibs.ResponseInput{
			Error: &errors.Error{
				Code:    errors.InvalidRequestCode,
				Message: "invalid namespace",
			},
		})
		return
	}

	warns, err := ah.usecases.StopMethod(namespace, method)
	if err != nil {
		resp.ToResponse(apilibs.ResponseInput{
			Error: &errors.Error{
				Code:    errors.InternalServerCode,
				Message: err.Error(),
			},
			Warnings: warns,
		})
		return
	}

	resp.ToResponse(apilibs.ResponseInput{
		StatusSuccess: int(errors.SuccessCode),
		Warnings:      warns,
	})
}

func (ah *ArrowHandler) KillMethod(c *gin.Context) {
	resp := apilibs.NewApiResponse(c)
	namespace := c.Param("namespace")
	method := c.Param("method")

	if !ah.apilib.IsNamespace(namespace, "") {
		resp.ToResponse(apilibs.ResponseInput{
			Error: &errors.Error{
				Code:    errors.InvalidRequestCode,
				Message: "invalid namespace",
			},
		})
		return
	}

	warns, err := ah.usecases.KillMethod(namespace, method)
	if err != nil {
		resp.ToResponse(apilibs.ResponseInput{
			Error: &errors.Error{
				Code:    errors.InternalServerCode,
				Message: err.Error(),
			},
			Warnings: warns,
		})
		return
	}

	resp.ToResponse(apilibs.ResponseInput{
		StatusSuccess: int(errors.SuccessCode),
		Warnings:      warns,
	})
}

// TODO: Implement method
func (ah *ArrowHandler) ListenChannel(c *gin.Context) {
}
