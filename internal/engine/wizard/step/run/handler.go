package run

import (
	"context"
	"errors"
	"fmt"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
)

var ErrNonZeroExit = errors.New("run: process exited with non-zero code")

type handler struct {
	runtime *runtime.Runtime
}

func NewHandler(rt *runtime.Runtime) wizstep.Handler {
	return &handler{runtime: rt}
}

func (h *handler) ShouldExecute(t domainstep.StepType) bool {
	return t == domainstep.StepTypeRun
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.Step,
) error {
	rs := s.(domainstep.RunStep)

	stepCtx := ctx
	var cancel context.CancelFunc
	if rs.Timeout > 0 {
		stepCtx, cancel = context.WithTimeout(ctx, rs.Timeout)
		defer cancel()
	}

	proc, err := h.runtime.
		Get(stepCtx, rs.Command).
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
