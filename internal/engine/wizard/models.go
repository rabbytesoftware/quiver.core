package wizard

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

type ExecutionRequest struct {
	Namespace domain.Namespace
	Method    string
	Variables map[string]string
	Steps     []step.Step
	WorkDir   string
}
