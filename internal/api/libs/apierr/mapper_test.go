package apierr_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/api/libs/apierr"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/stretchr/testify/assert"
)

func TestStatusAndMessage(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantMsg    string
	}{
		{apperrors.ErrNotFound, http.StatusNotFound, "not found"},
		{apperrors.ErrAlreadyExists, http.StatusConflict, "already exists"},
		{apperrors.ErrAlreadyRemoved, http.StatusConflict, "already removed"},
		{apperrors.ErrStateViolation, http.StatusUnprocessableEntity, "state violation"},
		{apperrors.ErrFetchFailed, http.StatusBadGateway, "fetch failed"},
		{apperrors.ErrInvalidNamespace, http.StatusBadRequest, "invalid namespace"},
		{apperrors.ErrDependentsExist, http.StatusUnprocessableEntity, "other arrows depend on this arrow"},
		{apperrors.ErrInvalidManifest, http.StatusUnprocessableEntity, "invalid manifest"},
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

func TestStatusAndMessage_WrappedErrors(t *testing.T) {
	wrapped := fmt.Errorf("catalog: %w", apperrors.ErrNotFound)
	status, msg := apierr.StatusAndMessage(wrapped)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, "not found", msg)
}
