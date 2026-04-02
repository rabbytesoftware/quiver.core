package run

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
)

var ErrNonZeroExit = errors.New("run: process exited with non-zero code")

type handler struct {
	runtime *runtime.Runtime
}

func NewHandler(
	rt *runtime.Runtime,
) wizstep.Handler[domainstep.RunStep] {
	return &handler{runtime: rt}
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.RunStep,
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

	proc, err := h.runtime.
		Get(stepCtx, s.Command.Resolve(req.OSArch.String())).
		WithShellWrap().
		WithWorkDir(req.WorkDir).
		WithEnv(req.Vars).
		Build()
	if err != nil {
		return err
	}

	if err := proc.Start(stepCtx); err != nil {
		h.runtime.Unregister(proc.ID())
		return err
	}

	if req.Tracker != nil {
		req.Tracker.SetKey(proc.Key())
	}

	if err := proc.Wait(stepCtx); err != nil {
		return err
	}

	if proc.ExitCode() != 0 {
		return fmt.Errorf("%w: %d", ErrNonZeroExit, proc.ExitCode())
	}

	return nil
}
