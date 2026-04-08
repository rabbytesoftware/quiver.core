package apierr

import "fmt"

type Error struct {
	Code    ErrorCode              `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details"`
}

func Throw(
	code ErrorCode,
	message string,
	details map[string]interface{},
) Error {
	return Error{
		Code:    code,
		Message: message,
		Details: details,
	}
}

func (e Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

func InvalidRequest(msg string) error {
	return Throw(InvalidRequestCode, msg, nil)
}

func Unauthorized(msg string) error {
	return Throw(UnauthorizedCode, msg, nil)
}

func PaymentRequired(msg string) error {
	return Throw(PaymentRequiredCode, msg, nil)
}

func Forbidden(msg string) error {
	return Throw(ForbiddenCode, msg, nil)
}

func NotFound(msg string) error {
	return Throw(NotFoundCode, msg, nil)
}

func MethodNotAllowed(msg string) error {
	return Throw(MethodNotAllowedCode, msg, nil)
}

func NotAcceptable(msg string) error {
	return Throw(NotAcceptableCode, msg, nil)
}

func ProxyAuthenticationRequired(msg string) error {
	return Throw(ProxyAuthenticationRequiredCode, msg, nil)
}

func RequestTimeout(msg string) error {
	return Throw(RequestTimeoutCode, msg, nil)
}

func Conflict(msg string) error {
	return Throw(ConflictCode, msg, nil)
}

func Gone(msg string) error {
	return Throw(GoneCode, msg, nil)
}

func LengthRequired(msg string) error {
	return Throw(LengthRequiredCode, msg, nil)
}

func PreconditionFailed(msg string) error {
	return Throw(PreconditionFailedCode, msg, nil)
}

func PayloadTooLarge(msg string) error {
	return Throw(PayloadTooLargeCode, msg, nil)
}

func URITooLong(msg string) error {
	return Throw(URITooLongCode, msg, nil)
}

func UnsupportedMediaType(msg string) error {
	return Throw(UnsupportedMediaTypeCode, msg, nil)
}

func RangeNotSatisfiable(msg string) error {
	return Throw(RangeNotSatisfiableCode, msg, nil)
}

func ExpectationFailed(msg string) error {
	return Throw(ExpectationFailedCode, msg, nil)
}

func ImATeapot(msg string) error {
	return Throw(ImATeapotCode, msg, nil)
}

func MisdirectedRequest(msg string) error {
	return Throw(MisdirectedRequestCode, msg, nil)
}

func UnprocessableEntity(msg string) error {
	return Throw(UnprocessableEntityCode, msg, nil)
}

func Locked(msg string) error {
	return Throw(LockedCode, msg, nil)
}

func FailedDependency(msg string) error {
	return Throw(FailedDependencyCode, msg, nil)
}

func TooEarly(msg string) error {
	return Throw(TooEarlyCode, msg, nil)
}

func UpgradeRequired(msg string) error {
	return Throw(UpgradeRequiredCode, msg, nil)
}

func PreconditionRequired(msg string) error {
	return Throw(PreconditionRequiredCode, msg, nil)
}

func TooManyRequests(msg string) error {
	return Throw(TooManyRequestsCode, msg, nil)
}

func RequestHeaderFieldsTooLarge(msg string) error {
	return Throw(RequestHeaderFieldsTooLargeCode, msg, nil)
}

func UnavailableForLegalReasons(msg string) error {
	return Throw(UnavailableForLegalReasonsCode, msg, nil)
}

func InternalServerError(msg string) error {
	return Throw(InternalServerCode, msg, nil)
}

func NotImplemented(msg string) error {
	return Throw(NotImplementedCode, msg, nil)
}

func BadGateway(msg string) error {
	return Throw(BadGatewayCode, msg, nil)
}

func ServiceUnavailable(msg string) error {
	return Throw(ServiceUnavailableCode, msg, nil)
}

func GatewayTimeout(msg string) error {
	return Throw(GatewayTimeoutCode, msg, nil)
}

func HTTPVersionError(msg string) error {
	return Throw(HTTPVersionErrorCode, msg, nil)
}

func VariantAlsoNegotiates(msg string) error {
	return Throw(VariantAlsoNegotiatesCode, msg, nil)
}

func InsufficientStorage(msg string) error {
	return Throw(InsufficientStorageCode, msg, nil)
}

func LoopDetected(msg string) error {
	return Throw(LoopDetectedCode, msg, nil)
}

func NotExtended(msg string) error {
	return Throw(NotExtendedCode, msg, nil)
}

func NetworkAuthenticationRequired(msg string) error {
	return Throw(NetworkAuthenticationRequiredCode, msg, nil)
}
