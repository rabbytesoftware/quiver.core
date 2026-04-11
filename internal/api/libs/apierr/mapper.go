package apierr

import (
	"errors"
	"net/http"

	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
)

// StatusAndMessage maps app-layer sentinel errors to an HTTP status code and
// human-readable message. Supports wrapped errors via errors.Is.
func StatusAndMessage(err error) (int, string) {
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, apperrors.ErrAlreadyExists):
		return http.StatusConflict, "already exists"
	case errors.Is(err, apperrors.ErrAlreadyRemoved):
		return http.StatusConflict, "already removed"
	case errors.Is(err, apperrors.ErrStateViolation):
		return http.StatusUnprocessableEntity, "state violation"
	case errors.Is(err, apperrors.ErrFetchFailed):
		return http.StatusBadGateway, "fetch failed"
	case errors.Is(err, apperrors.ErrInvalidNamespace):
		return http.StatusBadRequest, "invalid namespace"
	case errors.Is(err, apperrors.ErrDependentsExist):
		return http.StatusUnprocessableEntity, "other arrows depend on this arrow"
	case errors.Is(err, apperrors.ErrInvalidManifest):
		return http.StatusUnprocessableEntity, "invalid manifest"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}
