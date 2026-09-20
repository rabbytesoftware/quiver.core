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

// SetVersionOutdated returns a function that reconciles ns's runtime state with
// what a version check just found: Ready becomes Outdated when a newer release
// exists, and a version-drift Outdated becomes Ready again when it no longer
// does. The frontend's outdated badge reads ArrowRuntime.State, not
// Arrow.Outdated, so without this the catalog can record drift no client ever
// sees — and Outdated is a state an arrow cannot be run from, so the reverse
// direction is no less required than the forward one.
//
// It takes the answer rather than an instruction, and decides which command to
// send here, because which transitions are legal is the runtime aggregate's own
// business and must not leak into the arrow repository that calls this.
//
// The read comes first so the ordinary check — no drift, already Ready, which
// is almost every check — costs one read and writes nothing. Everything it
// declines to act on is a normal answer, not a failure: no aggregate at all
// (nobody installed this arrow), an arrow busy doing something else, or an
// Outdated some dependency sync put there, which a version check neither
// observed nor is entitled to clear. The commands' own Validate stays the
// authoritative guard for the window between that read and the send.
//
// It is a function over the aggregate rather than a method on Runtime for the
// same reason MarkPreinstalled is: the arrow repository needs it before
// runtime.New can be called at all, because runtime.New itself takes the arrow
// repository's MarkInstalled and friends.
//
// The send is a SendWait, and waiting here blocks nobody. Its one caller is the
// detached goroutine maybeCheckVersion launches, which belongs to no asynx
// worker pool; the cross-instance circular wait documented on
// internal/app/container.go's newAsynx needs an arrow worker blocked on a
// runtime send, which this deliberately is not. Waiting is also what keeps the
// runtime state durable and broadcast before the catalog record follows it.
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
