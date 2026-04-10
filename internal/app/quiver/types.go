package quiver

import (
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

var (
	ErrNotFound         = apperrors.ErrNotFound
	ErrAlreadyExists    = apperrors.ErrAlreadyExists
	ErrAlreadyRemoved   = apperrors.ErrAlreadyRemoved
	ErrStateViolation   = apperrors.ErrStateViolation
	ErrFetchFailed      = apperrors.ErrFetchFailed
	ErrInvalidNamespace = apperrors.ErrInvalidNamespace
)

type QuiverListDTO struct {
	Namespace   domain.Namespace `json:"namespace"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Tags        []string         `json:"tags"`
	Removed     bool             `json:"removed"`
}

type QuiverDetailDTO struct {
	Namespace domain.Namespace      `json:"namespace"`
	Manifest  domain.QuiverManifest `json:"manifest"`
	Removed   bool                  `json:"removed"`
}
