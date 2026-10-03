package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
)

type ShutdownDTO struct {
	PID  int      `json:"pid"`
	Exe  string   `json:"exe"`
	Args []string `json:"args"`
}

func ShutdownDTOFrom(
	info models.ShutdownInfo,
) ShutdownDTO {
	args := info.Args
	if args == nil {
		args = []string{}
	}

	return ShutdownDTO{PID: info.PID, Exe: info.Exe, Args: args}
}
