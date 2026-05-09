package run

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/runtime"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
)

var ErrNonZeroExit = errors.New("run: process exited with non-zero code")

type handler struct {
	rt runtime.Runtime
}

func NewHandler(
	rt runtime.Runtime,
) wizstep.Handler[domainstep.RunStep] {
	return &handler{rt: rt}
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

	config := runtime.NewConfig([]string{s.Command.Resolve(req.OSArch.String())})
	config.ShellWrap = true
	config.WorkDir = req.WorkDir
	config.Env = req.Vars

	proc, err := h.rt.Start(stepCtx, config)
	if err != nil {
		return err
	}

	if req.Emit != nil {
		req.Emit(models.Event{Kind: models.EventKindPID, PID: proc.PID()})
	}

	if err := proc.Wait(stepCtx); err != nil {
		return err
	}

	if proc.ExitCode() != 0 {
		return fmt.Errorf("%w: %d", ErrNonZeroExit, proc.ExitCode())
	}

	return nil
}
