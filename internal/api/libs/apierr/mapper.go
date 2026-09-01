package apierr

import (
	"errors"
	"net/http"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/engine/deptree"
)

// StatusAndMessage maps app-layer sentinel errors to an HTTP status code and
// human-readable message. Supports wrapped errors via errors.Is.
func StatusAndMessage(err error) (int, string) {
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, apperrors.ErrAlreadyExists):
		return http.StatusConflict, "already exists"
	case errors.Is(err, apperrors.ErrStateViolation):
		return http.StatusUnprocessableEntity, "state violation"
	case errors.Is(err, apperrors.ErrMethodNotFound):
		return http.StatusNotFound, "method not found"
	case errors.Is(err, apperrors.ErrFetchFailed):
		return http.StatusBadGateway, "fetch failed"
	case errors.Is(err, apperrors.ErrInvalidNamespace):
		return http.StatusBadRequest, "invalid namespace"
	case errors.Is(err, apperrors.ErrDependentsExist):
		return http.StatusUnprocessableEntity, "other arrows depend on this arrow"
	case errors.Is(err, apperrors.ErrPlatformNotSupported):
		return http.StatusUnprocessableEntity, "no target for the current platform"
	case errors.Is(err, apperrors.ErrMissingVariable):
		return http.StatusUnprocessableEntity, "required variable not provided"
	// The next two forward the error text instead of a constant message: the
	// name of the offending variable or field is the whole point of the
	// rejection, and the caller cannot act on the error without it.
	case errors.Is(err, apperrors.ErrReservedVariable):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, apperrors.ErrInvalidConfig):
		return http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, apperrors.ErrInvalidManifest):
		return http.StatusUnprocessableEntity, "invalid manifest"
	case errors.Is(err, deptree.ErrCyclicDependency):
		return http.StatusConflict, "cyclic dependency"
	default:
		return authStatusAndMessage(err)
	}
}

// authStatusAndMessage covers the device-pairing sentinels. Split out of
// StatusAndMessage's switch to keep that function under the cyclomatic
// complexity limit — every case here would otherwise count against it.
func authStatusAndMessage(err error) (int, string) {
	switch {
	case errors.Is(err, apperrors.ErrInvalidPairingCode):
		return http.StatusBadRequest, "invalid or expired pairing code"
	case errors.Is(err, apperrors.ErrUnauthorized):
		return http.StatusUnauthorized, "unauthorized"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}
