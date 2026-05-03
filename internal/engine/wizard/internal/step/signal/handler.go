package signal

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/runtime"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/step"
)

var (
	ErrNoProcess     = errors.New("signal: no process for namespace")
	ErrInvalidSignal = errors.New("signal: invalid signal kind")
)

type handler struct {
	rt runtime.Runtime
}

func NewHandler(
	rt runtime.Runtime,
) wizstep.Handler[domainstep.SignalStep] {
	return &handler{rt: rt}
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.SignalStep,
) error {
	sig := s.Signal.Resolve(req.OSArch.String())

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

	if req.PID <= 0 {
		return ErrNoProcess
	}

	if err := h.rt.SignalPID(stepCtx, req.PID, sig); err != nil {
		if errors.Is(err, runtime.ErrInvalidSignal) {
			return ErrInvalidSignal
		}
		return err
	}
	return nil
}
