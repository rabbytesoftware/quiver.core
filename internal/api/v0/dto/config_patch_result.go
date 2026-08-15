package dto

import "github.com/rabbytesoftware/quiver.core/internal/app/usecases"

// ConfigPatchResultDTO reports which configuration fields a patch persisted
// and which it refused, each rejection naming the field and the reason.
type ConfigPatchResultDTO struct {
	Applied  []string             `json:"applied"`
	Rejected []ConfigRejectionDTO `json:"rejected"`
}

// ConfigRejectionDTO is a single refused configuration field.
type ConfigRejectionDTO struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

// ConfigPatchResultDTOFrom maps a usecase patch result onto the wire shape.
func ConfigPatchResultDTOFrom(
	result usecases.PatchResult,
) ConfigPatchResultDTO {
	applied := result.Applied
	if applied == nil {
		applied = []string{}
	}

	rejected := make([]ConfigRejectionDTO, 0, len(result.Rejected))
	for _, fe := range result.Rejected {
		rejected = append(rejected, ConfigRejectionDTO{Key: fe.Key, Message: fe.Message})
	}

	return ConfigPatchResultDTO{Applied: applied, Rejected: rejected}
}
