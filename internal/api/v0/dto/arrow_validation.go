package dto

import "github.com/rabbytesoftware/quiver/internal/app/arrow"

// ValidationResultDTO is the response body for SEED /arrow/:ns/validate.
type ValidationResultDTO struct {
	Valid  bool                 `json:"valid"`
	Errors []ValidationErrorDTO `json:"errors,omitempty"`
}

// ValidationErrorDTO represents a single rule violation.
type ValidationErrorDTO struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// ValidationResultDTOFrom maps the app-layer result to the API DTO.
func ValidationResultDTOFrom(r *arrow.ValidationResult) ValidationResultDTO {
	if len(r.Errors) == 0 {
		return ValidationResultDTO{Valid: r.Valid}
	}
	errs := make([]ValidationErrorDTO, len(r.Errors))
	for i, e := range r.Errors {
		errs[i] = ValidationErrorDTO{
			Field:   e.Field,
			Rule:    e.Rule,
			Message: e.Message,
		}
	}
	return ValidationResultDTO{Valid: r.Valid, Errors: errs}
}
