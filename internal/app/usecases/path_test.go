package usecases_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func TestPathUsecase_StatusAndSetup(t *testing.T) {
	errBoom := errors.New("boom")
	shelf := wizardPkg.PathStatus{BinDir: "/home/u/.quiver/bin", OnPath: true, Configured: true, Files: []string{"/home/u/.zshrc"}}
	want := models.PathStatus{BinDir: "/home/u/.quiver/bin", OnPath: true, Configured: true, Files: []string{"/home/u/.zshrc"}}

	testCases := []struct {
		name string
		call func(usecases.PathUsecase) (models.PathStatus, error)
	}{
		{name: "status", call: func(uc usecases.PathUsecase) (models.PathStatus, error) { return uc.Status(context.Background()) }},
		{name: "setup", call: func(uc usecases.PathUsecase) (models.PathStatus, error) { return uc.Setup(context.Background()) }},
	}

	for _, tc := range testCases {
		t.Run(tc.name+" returns the wizard status", func(t *testing.T) {
			result := func(context.Context) (wizardPkg.PathStatus, error) { return shelf, nil }
			uc := usecases.NewPathUsecase(&mocks.Wizard{PathStatusFn: result, SetupPathFn: result})

			got, err := tc.call(uc)

			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
		t.Run(tc.name+" wraps the wizard error", func(t *testing.T) {
			failing := func(context.Context) (wizardPkg.PathStatus, error) { return wizardPkg.PathStatus{}, errBoom }
			uc := usecases.NewPathUsecase(&mocks.Wizard{PathStatusFn: failing, SetupPathFn: failing})

			_, err := tc.call(uc)

			require.ErrorIs(t, err, errBoom)
		})
	}
}
