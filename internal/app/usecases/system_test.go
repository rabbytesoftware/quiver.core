package usecases_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

func TestSystemUsecase_Shutdown_StopsAndReportsProcess(t *testing.T) {
	stopped := 0
	uc := usecases.NewSystemUsecase(func() { stopped++ })

	info, err := uc.Shutdown(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, stopped)
	assert.Equal(t, os.Getpid(), info.PID)
	assert.Equal(t, os.Args[1:], info.Args)
	assert.NotEmpty(t, info.Exe)
}

func TestSystemUsecase_Shutdown_NoStopFuncIsStateViolation(t *testing.T) {
	uc := usecases.NewSystemUsecase(nil)

	_, err := uc.Shutdown(context.Background())

	require.ErrorIs(t, err, apperrors.ErrStateViolation)
}
