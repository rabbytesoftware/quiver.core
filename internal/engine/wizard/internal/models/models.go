package models

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

type RunRequest struct {
	Namespace domain.Namespace
	Method    string
	Variables map[string]string
	Steps     []step.Step
	WorkDir   string
	// PID is a pre-existing OS process ID used during crash recovery when a
	// detached process is still alive. Zero means no pre-existing PID.
	PID int
}
