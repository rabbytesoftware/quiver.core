package advance

import (
	"context"
	"errors"
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	arrowcmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// maxWriteAttempts bounds how often a write to a row another writer changed
// under it is judged or sent again.
const maxWriteAttempts = 3

// Judge judges what is ahead of current; answered is false when it has no
// trustworthy answer.
type Judge func(current domain.Arrow) (available *domain.Available, answered bool, err error)

// RecordAvailable records judge's answer about ns's row. The answer is
// written only while the row still holds the Resolved it was judged against:
// the command refuses it otherwise, and a concurrent append to the row fails
// it with a version conflict. Either way the row is re-read and judged
// again, a bounded number of times, so an answer about a Resolved the row has
// already left (a row outdated right after its update) is never recorded.
func (a *advancer) RecordAvailable(
	ctx context.Context,
	ns domain.Namespace,
	judge Judge,
) (*domain.Available, bool, error) {
	for attempt := 1; ; attempt++ {
		current, err := a.axArrow.Get(ctx, ns.String())
		if err != nil {
			return nil, false, MapGetErr(err)
		}
		available, answered, err := judge(current)
		if err != nil || !answered {
			return nil, answered, err
		}
		if sameAvailable(current.Available, available) {
			return available, true, nil
		}

		_, err = a.axArrow.SendWait(ctx, arrowcmds.RecordAvailable{
			Namespace:      ns,
			Available:      available,
			JudgedResolved: current.Resolved,
		})
		if err == nil {
			return available, true, nil
		}
		if !retryableWrite(err) || attempt == maxWriteAttempts {
			return available, true, err
		}
	}
}

// MapGetErr classifies a failed read of a row found moments earlier: one
// forgotten in between is not found.
func MapGetErr(err error) error {
	if errors.Is(err, asynxModels.ErrNotFound) {
		return fmt.Errorf("row forgotten: %w", apperrors.ErrNotFound)
	}
	return err
}

// retryableWrite reports whether a rejected write is worth judging again:
// the row changed under it, either by a concurrent append or before a
// command's own check against what the writer read.
func retryableWrite(err error) bool {
	return errors.Is(err, asynxModels.ErrPipelineFailed) || errors.Is(err, asynxModels.ErrValidation)
}

func sameAvailable(
	a *domain.Available,
	b *domain.Available,
) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
