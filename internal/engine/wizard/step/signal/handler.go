package signal

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
)

var ErrNoProcess = errors.New("signal: no process for namespace")
var ErrInvalidSignal = errors.New("signal: invalid signal name")

type handler struct{}

func NewHandler() wizstep.Handler[domainstep.SignalStep] {
	return &handler{}
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.SignalStep,
) error {
	stepCtx := ctx
	var cancel context.CancelFunc
	if ts := s.Timeout.Resolve(req.OSArch.String()); ts != "" {
		d, err := time.ParseDuration(ts)
		if err != nil {
			return fmt.Errorf("invalid timeout %q: %w", ts, err)
		}
		stepCtx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}

	if req.Tracker == nil {
		return ErrNoProcess
	}
	proc, ok := req.Tracker.GetProcess()
	if !ok {
		return ErrNoProcess
	}

	sig, err := ParseSignal(string(s.Signal.Resolve(req.OSArch.String())))
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
