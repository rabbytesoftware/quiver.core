package wizard

import (
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStepError_Error(t *testing.T) {
	s := step.NewRunStep("test", "echo hi", false, "5s", true)
	cause := errors.New("process failed")
	e := &StepError{Index: 3, Step: s, Cause: cause}

	assert.Equal(t, "step 3 failed: process failed", e.Error())
}

func TestStepError_Unwrap(t *testing.T) {
	cause := errors.New("process failed")
	e := &StepError{Index: 0, Cause: cause}

	assert.True(t, errors.Is(e, cause))
}

func TestSentinelErrors(t *testing.T) {
	sentinels := []error{
		ErrUnknownStepType,
		ErrExecutionExists,
	}

	for i, e := range sentinels {
		require.NotNil(t, e, "sentinel at index %d is nil", i)
	}

	for i := range sentinels {
		for j := range sentinels {
			if i == j {
				continue
			}
			assert.NotEqual(t, sentinels[i], sentinels[j],
				"sentinels at index %d and %d are equal", i, j)
		}
	}
}
