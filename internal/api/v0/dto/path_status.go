package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
)

type PathStatusDTO struct {
	BinDir     string   `json:"bin_dir"`
	OnPath     bool     `json:"on_path"`
	Configured bool     `json:"configured"`
	Files      []string `json:"files"`
}

func PathStatusDTOFrom(
	status models.PathStatus,
) PathStatusDTO {
	files := status.Files
	if files == nil {
		files = []string{}
	}

	return PathStatusDTO{
		BinDir:     status.BinDir,
		OnPath:     status.OnPath,
		Configured: status.Configured,
		Files:      files,
	}
}
