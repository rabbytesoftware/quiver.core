package models

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type ValidationResult struct {
	Valid                bool
	Errors               []ValidationError
	SupportedPlatforms   []domain.OS
	UnsupportedPlatforms []domain.OS
}
