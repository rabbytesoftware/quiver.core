package runtimeinternal

import (
	"errors"

	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

type drainReport struct {
	superseded       bool
	checksumMismatch bool
}

func (r *drainReport) observe(
	evt wizardPkg.Event,
) {
	if evt.Kind != wizardPkg.EventKindStepFailed {
		return
	}
	if errors.Is(evt.Err, wizardPkg.ErrChecksumMismatch) {
		r.checksumMismatch = true
	}
}
