package errors_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
)

func TestStateViolationError_Message(t *testing.T) {
	testCases := []struct {
		name  string
		op    string
		state string
		want  string
	}{
		{"absent reads as not installed", "uninstall", "absent", "cannot uninstall: arrow is not installed"},
		{"empty state reads as not installed", "run", "", "cannot run: arrow is not installed"},
		{"names the current state", "stop", "ready", "cannot stop: arrow is ready"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, apperrors.NewStateViolation(tc.op, tc.state).Error())
		})
	}
}

func TestStateViolationError_IsStateViolation(t *testing.T) {
	err := apperrors.NewStateViolation("stop", "ready")
	assert.True(t, errors.Is(err, apperrors.ErrStateViolation),
		"must satisfy errors.Is for the sentinel so mapping still works")
}
