package errors_test

import (
	"errors"
	"fmt"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
)

func TestFromForget_Mapping(t *testing.T) {
	other := errors.New("boom")

	testCases := []struct {
		name         string
		in           error
		wantState    bool
		wantIdentity bool
	}{
		{name: "nil stays nil", in: nil, wantIdentity: true},
		{name: "in progress is a state violation", in: fmt.Errorf("x: %w", asynxModels.ErrForgetInProgress), wantState: true},
		{name: "failed passes through", in: asynxModels.ErrForgetFailed, wantIdentity: true},
		{name: "other passes through", in: other, wantIdentity: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := apperrors.FromForget(tc.in)
			assert.Equal(t, tc.wantState, errors.Is(got, apperrors.ErrStateViolation))
			if tc.wantIdentity {
				assert.Equal(t, tc.in, got)
			}
		})
	}
}
