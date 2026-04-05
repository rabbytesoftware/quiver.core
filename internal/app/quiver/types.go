package quiver

import (
	"errors"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrAlreadyExists    = errors.New("already exists")
	ErrAlreadyRemoved   = errors.New("already removed")
	ErrStateViolation   = errors.New("state violation")
	ErrFetchFailed      = errors.New("fetch failed")
	ErrInvalidNamespace = errors.New("invalid namespace")
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
