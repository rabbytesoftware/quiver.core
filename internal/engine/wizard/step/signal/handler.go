package signal

import (
	"context"
	"errors"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
)

var ErrNoProcess = errors.New("signal: no process for namespace")
var ErrInvalidSignal = errors.New("signal: invalid signal name")

type handler struct {
	runtime *runtime.Runtime
}

func NewHandler(rt *runtime.Runtime) wizstep.Handler {
	return &handler{runtime: rt}
}

func (h *handler) ShouldExecute(t domainstep.StepType) bool {
	return t == domainstep.StepTypeSignal
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.Step,
) error {
	ss := s.(domainstep.SignalStep)

	stepCtx := ctx
	var cancel context.CancelFunc
	if ss.Timeout > 0 {
		stepCtx, cancel = context.WithTimeout(ctx, ss.Timeout)
		defer cancel()
	}

	if req.Tracker == nil {
		return ErrNoProcess
	}
	key, ok := req.Tracker.GetKey()
	if !ok {
		return ErrNoProcess
	}

	proc, err := h.runtime.GetByKey(key)
	if err != nil {
		if errors.Is(err, models.ErrProcessNotFound) {
			return ErrNoProcess
		}
		return err
	}

	sig, err := ParseSignal(ss.Signal)
	if err != nil {
		return ErrInvalidSignal
	}

	if err := proc.Signal(sig); err != nil {
		return err
	}

	select {
	case <-proc.Done():
		return nil
	case <-stepCtx.Done():
		return stepCtx.Err()
	}
}
