package errors

import (
	"errors"
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
)

// FromForget maps the error of an asynx Forget to an app sentinel. An aggregate
// already being forgotten is a state violation; every other error passes
// through unchanged.
func FromForget(err error) error {
	if errors.Is(err, asynxModels.ErrForgetInProgress) {
		return fmt.Errorf("%w: %w", ErrStateViolation, err)
	}
	return err
}
