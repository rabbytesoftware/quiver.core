package wizard

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

type RunRequest struct {
	Namespace domain.Namespace
	Variables map[string]string
	Steps     []step.Step
	WorkDir   string
}
