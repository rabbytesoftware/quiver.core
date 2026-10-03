package usecases

import (
	"context"
	"fmt"
	"os"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
)

type SystemUsecase interface {
	Shutdown(
		ctx context.Context,
	) (models.ShutdownInfo, error)
}

type systemUsecase struct {
	stop func()
}

// NewSystemUsecase returns a SystemUsecase whose Shutdown asks the daemon to
// leave through stop, the same cancellation a SIGTERM reaches.
func NewSystemUsecase(
	stop func(),
) SystemUsecase {
	return &systemUsecase{stop: stop}
}

func (u *systemUsecase) Shutdown(
	_ context.Context,
) (models.ShutdownInfo, error) {
	if u.stop == nil {
		return models.ShutdownInfo{}, fmt.Errorf("shutdown: no stop function wired: %w", apperrors.ErrStateViolation)
	}

	exe, err := os.Executable()
	if err != nil {
		return models.ShutdownInfo{}, fmt.Errorf("shutdown: resolve executable: %w", err)
	}

	info := models.ShutdownInfo{PID: os.Getpid(), Exe: exe, Args: os.Args[1:]}
	u.stop()

	return info, nil
}
