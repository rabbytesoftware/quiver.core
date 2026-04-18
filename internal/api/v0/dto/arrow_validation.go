package dto

import "github.com/rabbytesoftware/quiver/internal/app/arrow"

// ValidationResultDTO is the response body for SEED /arrow/:ns/validate.
type ValidationResultDTO struct {
	Valid                bool                 `json:"valid"`
	Errors               []ValidationErrorDTO `json:"errors,omitempty"`
	SupportedPlatforms   []string             `json:"supported_platforms,omitempty"`
	UnsupportedPlatforms []string             `json:"unsupported_platforms,omitempty"`
}

// ValidationErrorDTO represents a single rule violation.
type ValidationErrorDTO struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// ValidationResultDTOFrom maps the app-layer result to the API DTO.
func ValidationResultDTOFrom(r *arrow.ValidationResult) ValidationResultDTO {
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
