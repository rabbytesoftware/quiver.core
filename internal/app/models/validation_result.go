package models

import "github.com/rabbytesoftware/quiver/internal/domain"

type ValidationResult struct {
	Valid                bool
	Errors               []ValidationError
	SupportedPlatforms   []domain.OS
	UnsupportedPlatforms []domain.OS
}
