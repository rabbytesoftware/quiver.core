package apierr_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs/apierr"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/engine/deptree"
)

func TestStatusAndMessage(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantMsg    string
	}{
		{apperrors.ErrNotFound, http.StatusNotFound, "not found"},
		{apperrors.ErrAlreadyExists, http.StatusConflict, "already exists"},
		{apperrors.ErrStateViolation, http.StatusUnprocessableEntity, "state violation"},
		{apperrors.ErrMethodNotFound, http.StatusNotFound, "method not found"},
		{apperrors.ErrFetchFailed, http.StatusBadGateway, "fetch failed"},
		{apperrors.ErrInvalidNamespace, http.StatusBadRequest, "invalid namespace"},
		{apperrors.ErrDependentsExist, http.StatusUnprocessableEntity, "other arrows depend on this arrow"},
		{apperrors.ErrPlatformNotSupported, http.StatusUnprocessableEntity, "no target for the current platform"},
		{apperrors.ErrMissingVariable, http.StatusUnprocessableEntity, "required variable not provided"},
		{apperrors.ErrInvalidManifest, http.StatusUnprocessableEntity, "invalid manifest"},
		{deptree.ErrCyclicDependency, http.StatusConflict, "cyclic dependency"},
		{errors.New("unexpected"), http.StatusInternalServerError, "internal error"},
	}

	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			status, msg := apierr.StatusAndMessage(tt.err)
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantMsg, msg)
		})
	}
}

func TestStatusAndMessage_StateViolationSurfacesContext(t *testing.T) {
	status, msg := apierr.StatusAndMessage(apperrors.NewStateViolation("uninstall", "absent"))
	assert.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Equal(t, "cannot uninstall: arrow is not installed", msg)

	// Wrapped in outer context, the StateViolationError message still surfaces.
	wrapped := fmt.Errorf("uninstall: %w", apperrors.NewStateViolation("stop", "ready"))
	status, msg = apierr.StatusAndMessage(wrapped)
	assert.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Equal(t, "cannot stop: arrow is ready", msg)
}

func TestStatusAndMessage_WrappedErrors(t *testing.T) {
	wrapped := fmt.Errorf("catalog: %w", apperrors.ErrNotFound)
	status, msg := apierr.StatusAndMessage(wrapped)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, "not found", msg)
}

// The offending name only exists in the wrapped text, so this mapping forwards
// the message rather than replacing it with a constant.
func TestStatusAndMessage_ReservedVariable_NamesTheVariable(t *testing.T) {
	err := fmt.Errorf("install: %w: %q", apperrors.ErrReservedVariable, "WORKDIR")

	status, msg := apierr.StatusAndMessage(err)

	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, msg, "WORKDIR")
	assert.Contains(t, msg, "reserved")
}

func TestStatusAndMessage_InvalidConfig_NamesTheField(t *testing.T) {
	err := fmt.Errorf("patch config: %w: %s", apperrors.ErrInvalidConfig, "logger.level")

	status, message := apierr.StatusAndMessage(err)

	assert.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Contains(t, message, "logger.level")
}
