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

// ErrNoWorkDir means a run step was asked to start without a workdir: it
// would run in the daemon's own working directory, so it never starts.
var ErrNoWorkDir = errors.New("run: no workdir")

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
	if req.WorkDir == "" {
		return ErrNoWorkDir
	}
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

	command := req.Expand(s.Command.Resolve(req.OSArch.String()))

	closeSurface, err := openSurface(req, s.UI)
	if err != nil {
		return err
	}
	defer closeSurface()

	config := runtime.NewConfig([]string{command})
	config.ShellWrap = true
	config.WorkDir = req.WorkDir

	proc, err := h.rt.Start(stepCtx, config)
	if err != nil {
		return err
	}

	emit(req, models.Event{Kind: models.EventKindPID, PID: proc.PID()})

	if err := proc.Wait(stepCtx); err != nil {
		return err
	}

	if proc.ExitCode() != 0 {
		return fmt.Errorf("%w: %d", ErrNonZeroExit, proc.ExitCode())
	}

	return nil
}
