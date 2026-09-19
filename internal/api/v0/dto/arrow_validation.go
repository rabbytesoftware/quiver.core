package dto

import "github.com/rabbytesoftware/quiver.core/internal/app/models"

// ValidationResultDTO is the response body for POST /arrow/:ns/manifest/validate.
type ValidationResultDTO struct {
	Valid                bool                 `json:"valid" yaml:"valid"`
	Errors               []ValidationErrorDTO `json:"errors,omitempty" yaml:"errors,omitempty"`
	SupportedPlatforms   []string             `json:"supported_platforms,omitempty" yaml:"supported_platforms,omitempty"`
	UnsupportedPlatforms []string             `json:"unsupported_platforms,omitempty" yaml:"unsupported_platforms,omitempty"`
}

type ValidationErrorDTO struct {
	Field   string `json:"field" yaml:"field"`
	Rule    string `json:"rule" yaml:"rule"`
	Message string `json:"message" yaml:"message"`
}

func ValidationResultDTOFrom(r *models.ValidationResult) ValidationResultDTO {
	supported := make([]string, 0, len(r.SupportedPlatforms))
	for _, os := range r.SupportedPlatforms {
		supported = append(supported, os.String())
	}

	unsupported := make([]string, 0, len(r.UnsupportedPlatforms))
	for _, os := range r.UnsupportedPlatforms {
		unsupported = append(unsupported, os.String())
	}

	if len(r.Errors) == 0 {
		return ValidationResultDTO{
			Valid:                r.Valid,
			SupportedPlatforms:   supported,
			UnsupportedPlatforms: unsupported,
		}
	}
	errs := make([]ValidationErrorDTO, len(r.Errors))
	for i, e := range r.Errors {
		errs[i] = ValidationErrorDTO{
			Field:   e.Field,
			Rule:    e.Rule,
			Message: e.Message,
		}
	}
	return ValidationResultDTO{
		Valid:                r.Valid,
		Errors:               errs,
		SupportedPlatforms:   supported,
		UnsupportedPlatforms: unsupported,
	}
}
