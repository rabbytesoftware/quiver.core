package apierr

type ErrorCode int

const (
	// 4xx Client Error
	InvalidRequestCode              ErrorCode = 400
	UnauthorizedCode                ErrorCode = 401
	PaymentRequiredCode             ErrorCode = 402
	ForbiddenCode                   ErrorCode = 403
	NotFoundCode                    ErrorCode = 404
	MethodNotAllowedCode            ErrorCode = 405
	NotAcceptableCode               ErrorCode = 406
	ProxyAuthenticationRequiredCode ErrorCode = 407
	RequestTimeoutCode              ErrorCode = 408
	ConflictCode                    ErrorCode = 409
	GoneCode                        ErrorCode = 410
	LengthRequiredCode              ErrorCode = 411
	PreconditionFailedCode          ErrorCode = 412
	PayloadTooLargeCode             ErrorCode = 413
	URITooLongCode                  ErrorCode = 414
	UnsupportedMediaTypeCode        ErrorCode = 415
	RangeNotSatisfiableCode         ErrorCode = 416
	ExpectationFailedCode           ErrorCode = 417
	ImATeapotCode                   ErrorCode = 418
	MisdirectedRequestCode          ErrorCode = 421
	UnprocessableEntityCode         ErrorCode = 422
	LockedCode                      ErrorCode = 423
	FailedDependencyCode            ErrorCode = 424
	TooEarlyCode                    ErrorCode = 425
	UpgradeRequiredCode             ErrorCode = 426
	PreconditionRequiredCode        ErrorCode = 428
	TooManyRequestsCode             ErrorCode = 429
	RequestHeaderFieldsTooLargeCode ErrorCode = 431
	UnavailableForLegalReasonsCode  ErrorCode = 451

	// 5xx Server Error
	InternalServerCode                ErrorCode = 500
	NotImplementedCode                ErrorCode = 501
	BadGatewayCode                    ErrorCode = 502
	ServiceUnavailableCode            ErrorCode = 503
	GatewayTimeoutCode                ErrorCode = 504
	HTTPVersionErrorCode              ErrorCode = 505
	VariantAlsoNegotiatesCode         ErrorCode = 506
	InsufficientStorageCode           ErrorCode = 507
	LoopDetectedCode                  ErrorCode = 508
	NotExtendedCode                   ErrorCode = 510
	NetworkAuthenticationRequiredCode ErrorCode = 511
)
