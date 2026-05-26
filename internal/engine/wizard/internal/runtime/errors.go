package runtime

import (
	"errors"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/runtime/internal/models"
)

var (
	ErrUnsupportedOS = errors.New("unsupported operating system")
	ErrInvalidSignal = models.ErrInvalidSignal
)

// Re-exported process-level errors for callers that check via errors.Is.
var (
	ErrEmptyCommand   = models.ErrEmptyCommand
	ErrInvalidState   = models.ErrInvalidState
	ErrNoProcess      = models.ErrNoProcess
	ErrKillTimeout    = models.ErrKillTimeout
	ErrInvalidTimeout = models.ErrInvalidTimeout
)
