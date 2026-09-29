package usecases_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf"
)

type stubShelf struct {
	statusResult shelf.PathStatus
	statusErr    error
	setupResult  shelf.PathStatus
	setupErr     error
}

func (s *stubShelf) Apply(
	context.Context,
	domain.Namespace,
	string,
	domain.Expose,
	domain.ArrowMedia,
) (shelf.Applied, error) {
	return shelf.Applied{}, nil
}

func (s *stubShelf) Remove(
	context.Context,
	domain.Namespace,
) error {
	return nil
}

func (s *stubShelf) PathStatus(
	context.Context,
) (shelf.PathStatus, error) {
	return s.statusResult, s.statusErr
}

func (s *stubShelf) SetupPath(
	context.Context,
) (shelf.PathStatus, error) {
	return s.setupResult, s.setupErr
}

func TestPathUsecase_Status_ReturnsShelfStatus(t *testing.T) {
	status := shelf.PathStatus{BinDir: "/home/u/.quiver/bin", OnPath: true, Configured: true, Files: []string{"/home/u/.zshrc"}}
	uc := usecases.NewPathUsecase(&stubShelf{statusResult: status})

	got, err := uc.Status(context.Background())

	require.NoError(t, err)
	assert.Equal(t, models.PathStatus{BinDir: "/home/u/.quiver/bin", OnPath: true, Configured: true, Files: []string{"/home/u/.zshrc"}}, got)
}

func TestPathUsecase_Status_WrapsError(t *testing.T) {
	errBoom := errors.New("boom")
	uc := usecases.NewPathUsecase(&stubShelf{statusErr: errBoom})

	_, err := uc.Status(context.Background())

	require.ErrorIs(t, err, errBoom)
}

func TestPathUsecase_Setup_ReturnsShelfStatus(t *testing.T) {
	status := shelf.PathStatus{BinDir: "/home/u/.quiver/bin", OnPath: true, Configured: true, Files: []string{"/home/u/.zshrc"}}
	uc := usecases.NewPathUsecase(&stubShelf{setupResult: status})

	got, err := uc.Setup(context.Background())

	require.NoError(t, err)
	assert.Equal(t, models.PathStatus{BinDir: "/home/u/.quiver/bin", OnPath: true, Configured: true, Files: []string{"/home/u/.zshrc"}}, got)
}

func TestPathUsecase_Setup_WrapsError(t *testing.T) {
	errBoom := errors.New("boom")
	uc := usecases.NewPathUsecase(&stubShelf{setupErr: errBoom})

	_, err := uc.Setup(context.Background())

	require.ErrorIs(t, err, errBoom)
}
