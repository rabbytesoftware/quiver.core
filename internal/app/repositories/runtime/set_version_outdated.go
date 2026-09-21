package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	runtimecmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// SetVersionOutdated reconciles ns's runtime state with what a version check
// found: Ready becomes Outdated when newer exists, and reverses when it no
// longer does -- the frontend badge reads ArrowRuntime.State, not
// Arrow.Outdated, so this is what makes drift visible at all.
//
// A function over the aggregate rather than a method on Runtime, the same
// reason as MarkPreinstalled: the arrow repository needs it before
// runtime.New can be constructed.
//
// SendWait's blocking is safe here: its only caller is a detached goroutine
// outside any asynx worker pool, so it cannot recreate the cross-instance
// circular wait documented on container.go's newAsynx.
func SetVersionOutdated(
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) func(ctx context.Context, ns domain.Namespace, outdated bool) error {
	return func(ctx context.Context, ns domain.Namespace, outdated bool) error {
		cmd, err := versionOutdatedCommand(ctx, axRuntime, ns, outdated)
		if err != nil || cmd == nil {
			return err
		}

		if _, sendErr := axRuntime.SendWait(ctx, cmd); sendErr != nil {
			if errors.Is(sendErr, asynxModels.ErrValidation) ||
				errors.Is(sendErr, asynxModels.ErrPipelineFailed) {
				return fmt.Errorf("set version outdated %s: %w", ns, apperrors.ErrStateViolation)
			}
			return fmt.Errorf("set version outdated %s: %w", ns, sendErr)
		}

		return nil
	}
}

// versionOutdatedCommand picks the command ns's current state needs to reach
// the drift answer, or nil when it is already there or is none of a version
// check's business.
func versionOutdatedCommand(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
	outdated bool,
) (asynxModels.Command[domainRuntime.ArrowRuntime], error) {
	current, err := axRuntime.Get(ctx, ns.String())
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("set version outdated %s: %w", ns, err)
	}

	if outdated {
		if current.State != domain.ArrowStateReady {
			return nil, nil
		}
		return runtimecmds.MarkVersionOutdated{Namespace: ns}, nil
	}

	if current.State != domain.ArrowStateOutdated || current.PendingDepSync != nil {
		return nil, nil
	}

	return runtimecmds.ClearVersionOutdated{Namespace: ns}, nil
}
