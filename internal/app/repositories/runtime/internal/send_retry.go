package runtimeinternal

import (
	"context"
	"errors"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

const (
	conflictRetryAttempts = 5
	conflictRetryBackoff  = 10 * time.Millisecond
)

// sendRetryingConflicts resends cmd when it loses an optimistic-concurrency
// race on the aggregate. The drain goroutine and the readiness probe both
// write one execution's aggregate, so either can validate against a version
// the other has just appended to; the loser's command was never written and
// asynx asks the caller to send it again from scratch. Every resend is
// revalidated, so a superseded execution still fails with ErrValidation and
// stops here.
func sendRetryingConflicts(
	ctx context.Context,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	cmd asynxModels.Command[domainRuntime.ArrowRuntime],
) error {
	wait := conflictRetryBackoff
	for attempt := 1; ; attempt++ {
		_, err := axRuntime.Send(ctx, cmd)
		if !lostWriteRace(err) || attempt == conflictRetryAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(wait):
		}
		wait *= 2
	}
}

// lostWriteRace excludes ErrDispatcherClosed, which asynx wraps in
// ErrPipelineFailed only after the event was already appended.
func lostWriteRace(err error) bool {
	return errors.Is(err, asynxModels.ErrPipelineFailed) &&
		!errors.Is(err, asynxModels.ErrDispatcherClosed)
}
