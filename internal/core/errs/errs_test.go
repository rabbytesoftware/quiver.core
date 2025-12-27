package errors_test

import (
	"reflect"
	"testing"

	errs "github.com/rabbytesoftware/quiver/internal/core/errs"
)

func TestThrow(t *testing.T) {
	tests := []struct {
		name    string
		code    errs.ErrorCode
		message string
		details map[string]interface{}
	}{
		{
			name:    "with details",
			code:    errs.NotFoundCode,
			message: "resource not found",
			details: map[string]interface{}{"id": "123"},
		},
		{
			name:    "without details",
			code:    errs.InternalServerCode,
			message: "internal error",
			details: nil,
		},
		{
			name:    "empty message",
			code:    errs.SuccessCode,
			message: "",
			details: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errs.Throw(tt.code, tt.message, tt.details)
			if err.Code != tt.code {
				t.Errorf("Expected code %d, got %d", tt.code, err.Code)
			}
			if err.Message != tt.message {
				t.Errorf("Expected message %s, got %s", tt.message, err.Message)
			}
			if !reflect.DeepEqual(err.Details, tt.details) {
				t.Errorf("Expected details %v, got %v", tt.details, err.Details)
			}
		})
	}
}

func TestError_Error(t *testing.T) {
	tests := []struct {
		name    string
		code    errs.ErrorCode
		message string
		want    string
	}{
		{
			name:    "not found error",
			code:    errs.NotFoundCode,
			message: "resource not found",
			want:    "404: resource not found",
		},
		{
			name:    "internal server error",
			code:    errs.InternalServerCode,
			message: "something went wrong",
			want:    "500: something went wrong",
		},
		{
			name:    "success code",
			code:    errs.SuccessCode,
			message: "operation successful",
			want:    "200: operation successful",
		},
		{
			name:    "empty message",
			code:    errs.ConflictCode,
			message: "",
			want:    "409: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errs.Error{
				Code:    tt.code,
				Message: tt.message,
			}
			got := err.Error()
			if got != tt.want {
				t.Errorf("Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSuccess(t *testing.T) {
	msg := "operation successful"
	err := errs.Success(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.SuccessCode {
		t.Errorf("Expected code %d, got %d", errs.SuccessCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestCreated(t *testing.T) {
	msg := "resource created"
	err := errs.Created(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.CreatedCode {
		t.Errorf("Expected code %d, got %d", errs.CreatedCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestAccepted(t *testing.T) {
	msg := "request accepted"
	err := errs.Accepted(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.AcceptedCode {
		t.Errorf("Expected code %d, got %d", errs.AcceptedCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestNonAuthoritativeInformation(t *testing.T) {
	msg := "non authoritative"
	err := errs.NonAuthoritativeInformation(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.NonAuthoritativeInformationCode {
		t.Errorf("Expected code %d, got %d", errs.NonAuthoritativeInformationCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestNoContent(t *testing.T) {
	msg := "no content"
	err := errs.NoContent(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.NoContentCode {
		t.Errorf("Expected code %d, got %d", errs.NoContentCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestResetContent(t *testing.T) {
	msg := "reset content"
	err := errs.ResetContent(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.ResetContentCode {
		t.Errorf("Expected code %d, got %d", errs.ResetContentCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestPartialContent(t *testing.T) {
	msg := "partial content"
	err := errs.PartialContent(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.PartialContentCode {
		t.Errorf("Expected code %d, got %d", errs.PartialContentCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestMultiStatus(t *testing.T) {
	msg := "multi status"
	err := errs.MultiStatus(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.MultiStatusCode {
		t.Errorf("Expected code %d, got %d", errs.MultiStatusCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestAlreadyReported(t *testing.T) {
	msg := "already reported"
	err := errs.AlreadyReported(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.AlreadyReportedCode {
		t.Errorf("Expected code %d, got %d", errs.AlreadyReportedCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestIMUsed(t *testing.T) {
	msg := "IM used"
	err := errs.IMUsed(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.IMUsedCode {
		t.Errorf("Expected code %d, got %d", errs.IMUsedCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestInvalidRequest(t *testing.T) {
	msg := "invalid request"
	err := errs.InvalidRequest(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.InvalidRequestCode {
		t.Errorf("Expected code %d, got %d", errs.InvalidRequestCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestUnauthorized(t *testing.T) {
	msg := "unauthorized"
	err := errs.Unauthorized(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.UnauthorizedCode {
		t.Errorf("Expected code %d, got %d", errs.UnauthorizedCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestPaymentRequired(t *testing.T) {
	msg := "payment required"
	err := errs.PaymentRequired(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.PaymentRequiredCode {
		t.Errorf("Expected code %d, got %d", errs.PaymentRequiredCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestForbidden(t *testing.T) {
	msg := "forbidden"
	err := errs.Forbidden(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.ForbiddenCode {
		t.Errorf("Expected code %d, got %d", errs.ForbiddenCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestNotFound(t *testing.T) {
	msg := "not found"
	err := errs.NotFound(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.NotFoundCode {
		t.Errorf("Expected code %d, got %d", errs.NotFoundCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	msg := "method not allowed"
	err := errs.MethodNotAllowed(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.MethodNotAllowedCode {
		t.Errorf("Expected code %d, got %d", errs.MethodNotAllowedCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestNotAcceptable(t *testing.T) {
	msg := "not acceptable"
	err := errs.NotAcceptable(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.NotAcceptableCode {
		t.Errorf("Expected code %d, got %d", errs.NotAcceptableCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestProxyAuthenticationRequired(t *testing.T) {
	msg := "proxy authentication required"
	err := errs.ProxyAuthenticationRequired(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.ProxyAuthenticationRequiredCode {
		t.Errorf("Expected code %d, got %d", errs.ProxyAuthenticationRequiredCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestRequestTimeout(t *testing.T) {
	msg := "request timeout"
	err := errs.RequestTimeout(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.RequestTimeoutCode {
		t.Errorf("Expected code %d, got %d", errs.RequestTimeoutCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestConflict(t *testing.T) {
	msg := "conflict"
	err := errs.Conflict(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.ConflictCode {
		t.Errorf("Expected code %d, got %d", errs.ConflictCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestGone(t *testing.T) {
	msg := "gone"
	err := errs.Gone(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.GoneCode {
		t.Errorf("Expected code %d, got %d", errs.GoneCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestLengthRequired(t *testing.T) {
	msg := "length required"
	err := errs.LengthRequired(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.LengthRequiredCode {
		t.Errorf("Expected code %d, got %d", errs.LengthRequiredCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestPreconditionFailed(t *testing.T) {
	msg := "precondition failed"
	err := errs.PreconditionFailed(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.PreconditionFailedCode {
		t.Errorf("Expected code %d, got %d", errs.PreconditionFailedCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestPayloadTooLarge(t *testing.T) {
	msg := "payload too large"
	err := errs.PayloadTooLarge(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.PayloadTooLargeCode {
		t.Errorf("Expected code %d, got %d", errs.PayloadTooLargeCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestURITooLong(t *testing.T) {
	msg := "URI too long"
	err := errs.URITooLong(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.URITooLongCode {
		t.Errorf("Expected code %d, got %d", errs.URITooLongCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestUnsupportedMediaType(t *testing.T) {
	msg := "unsupported media type"
	err := errs.UnsupportedMediaType(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.UnsupportedMediaTypeCode {
		t.Errorf("Expected code %d, got %d", errs.UnsupportedMediaTypeCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestRangeNotSatisfiable(t *testing.T) {
	msg := "range not satisfiable"
	err := errs.RangeNotSatisfiable(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.RangeNotSatisfiableCode {
		t.Errorf("Expected code %d, got %d", errs.RangeNotSatisfiableCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestExpectationFailed(t *testing.T) {
	msg := "expectation failed"
	err := errs.ExpectationFailed(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.ExpectationFailedCode {
		t.Errorf("Expected code %d, got %d", errs.ExpectationFailedCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestImATeapot(t *testing.T) {
	msg := "I'm a teapot"
	err := errs.ImATeapot(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.ImATeapotCode {
		t.Errorf("Expected code %d, got %d", errs.ImATeapotCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestMisdirectedRequest(t *testing.T) {
	msg := "misdirected request"
	err := errs.MisdirectedRequest(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.MisdirectedRequestCode {
		t.Errorf("Expected code %d, got %d", errs.MisdirectedRequestCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestUnprocessableEntity(t *testing.T) {
	msg := "unprocessable entity"
	err := errs.UnprocessableEntity(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.UnprocessableEntityCode {
		t.Errorf("Expected code %d, got %d", errs.UnprocessableEntityCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestLocked(t *testing.T) {
	msg := "locked"
	err := errs.Locked(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.LockedCode {
		t.Errorf("Expected code %d, got %d", errs.LockedCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestFailedDependency(t *testing.T) {
	msg := "failed dependency"
	err := errs.FailedDependency(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.FailedDependencyCode {
		t.Errorf("Expected code %d, got %d", errs.FailedDependencyCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestTooEarly(t *testing.T) {
	msg := "too early"
	err := errs.TooEarly(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.TooEarlyCode {
		t.Errorf("Expected code %d, got %d", errs.TooEarlyCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestUpgradeRequired(t *testing.T) {
	msg := "upgrade required"
	err := errs.UpgradeRequired(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.UpgradeRequiredCode {
		t.Errorf("Expected code %d, got %d", errs.UpgradeRequiredCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestPreconditionRequired(t *testing.T) {
	msg := "precondition required"
	err := errs.PreconditionRequired(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.PreconditionRequiredCode {
		t.Errorf("Expected code %d, got %d", errs.PreconditionRequiredCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestTooManyRequests(t *testing.T) {
	msg := "too many requests"
	err := errs.TooManyRequests(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.TooManyRequestsCode {
		t.Errorf("Expected code %d, got %d", errs.TooManyRequestsCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestRequestHeaderFieldsTooLarge(t *testing.T) {
	msg := "request header fields too large"
	err := errs.RequestHeaderFieldsTooLarge(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.RequestHeaderFieldsTooLargeCode {
		t.Errorf("Expected code %d, got %d", errs.RequestHeaderFieldsTooLargeCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestUnavailableForLegalReasons(t *testing.T) {
	msg := "unavailable for legal reasons"
	err := errs.UnavailableForLegalReasons(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.UnavailableForLegalReasonsCode {
		t.Errorf("Expected code %d, got %d", errs.UnavailableForLegalReasonsCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestInternalServerError(t *testing.T) {
	msg := "internal server error"
	err := errs.InternalServerError(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.InternalServerCode {
		t.Errorf("Expected code %d, got %d", errs.InternalServerCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestNotImplemented(t *testing.T) {
	msg := "not implemented"
	err := errs.NotImplemented(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.NotImplementedCode {
		t.Errorf("Expected code %d, got %d", errs.NotImplementedCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestBadGateway(t *testing.T) {
	msg := "bad gateway"
	err := errs.BadGateway(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.BadGatewayCode {
		t.Errorf("Expected code %d, got %d", errs.BadGatewayCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestServiceUnavailable(t *testing.T) {
	msg := "service unavailable"
	err := errs.ServiceUnavailable(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.ServiceUnavailableCode {
		t.Errorf("Expected code %d, got %d", errs.ServiceUnavailableCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestGatewayTimeout(t *testing.T) {
	msg := "gateway timeout"
	err := errs.GatewayTimeout(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.GatewayTimeoutCode {
		t.Errorf("Expected code %d, got %d", errs.GatewayTimeoutCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestHTTPVersionError(t *testing.T) {
	msg := "HTTP version error"
	err := errs.HTTPVersionError(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.HTTPVersionErrorCode {
		t.Errorf("Expected code %d, got %d", errs.HTTPVersionErrorCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestVariantAlsoNegotiates(t *testing.T) {
	msg := "variant also negotiates"
	err := errs.VariantAlsoNegotiates(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.VariantAlsoNegotiatesCode {
		t.Errorf("Expected code %d, got %d", errs.VariantAlsoNegotiatesCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestInsufficientStorage(t *testing.T) {
	msg := "insufficient storage"
	err := errs.InsufficientStorage(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.InsufficientStorageCode {
		t.Errorf("Expected code %d, got %d", errs.InsufficientStorageCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestLoopDetected(t *testing.T) {
	msg := "loop detected"
	err := errs.LoopDetected(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.LoopDetectedCode {
		t.Errorf("Expected code %d, got %d", errs.LoopDetectedCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestNotExtended(t *testing.T) {
	msg := "not extended"
	err := errs.NotExtended(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.NotExtendedCode {
		t.Errorf("Expected code %d, got %d", errs.NotExtendedCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}

func TestNetworkAuthenticationRequired(t *testing.T) {
	msg := "network authentication required"
	err := errs.NetworkAuthenticationRequired(msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	e, ok := err.(errs.Error)
	if !ok {
		t.Fatalf("Expected errs.Error, got %T", err)
	}
	if e.Code != errs.NetworkAuthenticationRequiredCode {
		t.Errorf("Expected code %d, got %d", errs.NetworkAuthenticationRequiredCode, e.Code)
	}
	if e.Message != msg {
		t.Errorf("Expected message %s, got %s", msg, e.Message)
	}
}
