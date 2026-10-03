package errors_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestReleaseError_Message(t *testing.T) {
	err := apperrors.NewReleaseError(apperrors.ReleaseNoAsset, errors.New("host said no"))

	assert.Equal(t, "release unresolved: no_asset: host said no", err.Error())
}

func TestReleaseError_IsTheSentinelAndItsCause(t *testing.T) {
	cause := errors.New("connection refused")

	err := fmt.Errorf("variable %q: %w", "X", apperrors.NewReleaseError(apperrors.ReleaseOffline, cause))

	assert.ErrorIs(t, err, apperrors.ErrReleaseUnresolved)
	assert.ErrorIs(t, err, cause)
	var re *apperrors.ReleaseError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, apperrors.ReleaseOffline, re.Kind)
}
