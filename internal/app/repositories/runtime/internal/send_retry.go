package runtimeinternal

import (
	"context"
	"errors"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	eventstoreSqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

const (
	conflictRetryAttempts = 5
	conflictRetryBackoff  = 10 * time.Millisecond
)

// sendRetryingConflicts resends cmd when it loses an optimistic-concurrency
// race on the aggregate. The drain goroutine and the readiness probe both
// write one execution's aggregate, so either can validate against a version
// the other has just appended to. Only a version conflict is retried, because
// it alone guarantees the append did not happen; any other error is returned
// as is, since ErrPipelineFailed can also follow an append that committed
// (a failed snapshot write). Each resend is revalidated, so a superseded
// execution still stops on ErrValidation. Callers must still pass only
// commands that are idempotent under their ExecutionID guard.
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

// lostWriteRace still rules out ErrDispatcherClosed, which asynx reports only
// after the event was appended.
func lostWriteRace(
	err error,
) bool {
	return errors.Is(err, eventstoreSqlite.ErrVersionConflict) &&
		!errors.Is(err, asynxModels.ErrDispatcherClosed)
}
