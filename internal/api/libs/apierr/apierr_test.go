package apierr_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs/apierr"
)

func TestThrow_SetsFields(t *testing.T) {
	err := apierr.Throw(apierr.NotFoundCode, "not found", nil)

	assert.Equal(t, apierr.NotFoundCode, err.Code)
	assert.Equal(t, "not found", err.Message)
	assert.Nil(t, err.Details)
}

func TestError_FormatsCodeAndMessage(t *testing.T) {
	err := apierr.Throw(apierr.NotFoundCode, "resource missing", nil)

	assert.Equal(t, "404: resource missing", err.Error())
}

func TestThrow_WithDetails(t *testing.T) {
	details := map[string]interface{}{"field": "id"}
	err := apierr.Throw(apierr.UnprocessableEntityCode, "validation failed", details)

	assert.Equal(t, details, err.Details)
}

func TestConvenienceConstructors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code apierr.ErrorCode
	}{
		{"InvalidRequest", apierr.InvalidRequest("bad"), apierr.InvalidRequestCode},
		{"Unauthorized", apierr.Unauthorized("unauth"), apierr.UnauthorizedCode},
		{"PaymentRequired", apierr.PaymentRequired("pay"), apierr.PaymentRequiredCode},
		{"Forbidden", apierr.Forbidden("forbidden"), apierr.ForbiddenCode},
		{"NotFound", apierr.NotFound("missing"), apierr.NotFoundCode},
		{"MethodNotAllowed", apierr.MethodNotAllowed("method"), apierr.MethodNotAllowedCode},
		{"NotAcceptable", apierr.NotAcceptable("accept"), apierr.NotAcceptableCode},
		{"ProxyAuthenticationRequired", apierr.ProxyAuthenticationRequired("proxy"), apierr.ProxyAuthenticationRequiredCode},
		{"RequestTimeout", apierr.RequestTimeout("timeout"), apierr.RequestTimeoutCode},
		{"Conflict", apierr.Conflict("dup"), apierr.ConflictCode},
		{"Gone", apierr.Gone("gone"), apierr.GoneCode},
		{"LengthRequired", apierr.LengthRequired("length"), apierr.LengthRequiredCode},
		{"PreconditionFailed", apierr.PreconditionFailed("precond"), apierr.PreconditionFailedCode},
		{"PayloadTooLarge", apierr.PayloadTooLarge("large"), apierr.PayloadTooLargeCode},
		{"URITooLong", apierr.URITooLong("uri"), apierr.URITooLongCode},
		{"UnsupportedMediaType", apierr.UnsupportedMediaType("media"), apierr.UnsupportedMediaTypeCode},
		{"RangeNotSatisfiable", apierr.RangeNotSatisfiable("range"), apierr.RangeNotSatisfiableCode},
		{"ExpectationFailed", apierr.ExpectationFailed("expect"), apierr.ExpectationFailedCode},
		{"ImATeapot", apierr.ImATeapot("teapot"), apierr.ImATeapotCode},
		{"MisdirectedRequest", apierr.MisdirectedRequest("misdirected"), apierr.MisdirectedRequestCode},
		{"UnprocessableEntity", apierr.UnprocessableEntity("unprocessable"), apierr.UnprocessableEntityCode},
		{"Locked", apierr.Locked("locked"), apierr.LockedCode},
		{"FailedDependency", apierr.FailedDependency("dep"), apierr.FailedDependencyCode},
		{"TooEarly", apierr.TooEarly("early"), apierr.TooEarlyCode},
		{"UpgradeRequired", apierr.UpgradeRequired("upgrade"), apierr.UpgradeRequiredCode},
		{"PreconditionRequired", apierr.PreconditionRequired("precond"), apierr.PreconditionRequiredCode},
		{"TooManyRequests", apierr.TooManyRequests("many"), apierr.TooManyRequestsCode},
		{"RequestHeaderFieldsTooLarge", apierr.RequestHeaderFieldsTooLarge("headers"), apierr.RequestHeaderFieldsTooLargeCode},
		{"UnavailableForLegalReasons", apierr.UnavailableForLegalReasons("legal"), apierr.UnavailableForLegalReasonsCode},
		{"InternalServerError", apierr.InternalServerError("boom"), apierr.InternalServerCode},
		{"NotImplemented", apierr.NotImplemented("noimpl"), apierr.NotImplementedCode},
		{"BadGateway", apierr.BadGateway("gateway"), apierr.BadGatewayCode},
		{"ServiceUnavailable", apierr.ServiceUnavailable("unavail"), apierr.ServiceUnavailableCode},
		{"GatewayTimeout", apierr.GatewayTimeout("gwtimeout"), apierr.GatewayTimeoutCode},
		{"HTTPVersionError", apierr.HTTPVersionError("old"), apierr.HTTPVersionErrorCode},
		{"VariantAlsoNegotiates", apierr.VariantAlsoNegotiates("variant"), apierr.VariantAlsoNegotiatesCode},
		{"InsufficientStorage", apierr.InsufficientStorage("storage"), apierr.InsufficientStorageCode},
		{"LoopDetected", apierr.LoopDetected("loop"), apierr.LoopDetectedCode},
		{"NotExtended", apierr.NotExtended("extend"), apierr.NotExtendedCode},
		{"NetworkAuthenticationRequired", apierr.NetworkAuthenticationRequired("netauth"), apierr.NetworkAuthenticationRequiredCode},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var apiErr apierr.Error
			require.ErrorAs(t, tc.err, &apiErr)
			assert.Equal(t, tc.code, apiErr.Code)
		})
	}
}
