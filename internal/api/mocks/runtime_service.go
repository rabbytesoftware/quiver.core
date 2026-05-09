package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type RuntimeService struct {
	InstallErr          error
	UninstallErr        error
	ExecuteErr          error
	StopErr             error
	RuntimeExistsResult bool
	RuntimeExistsErr    error
}

func (m *RuntimeService) Install(
	_ context.Context,
	_ domain.Namespace,
	_ map[string]string,
) error {
	return m.InstallErr
}

func (m *RuntimeService) Uninstall(
	_ context.Context,
	_ domain.Namespace,
	_ map[string]string,
) error {
	return m.UninstallErr
}

func (m *RuntimeService) Execute(
	_ context.Context,
	_ domain.Namespace,
	_ string,
	_ map[string]string,
) error {
	return m.ExecuteErr
}

func (m *RuntimeService) Stop(
	_ context.Context,
	_ domain.Namespace,
) error {
	return m.StopErr
}

func (m *RuntimeService) RuntimeExists(
	_ context.Context,
	_ domain.Namespace,
) (bool, error) {
	return m.RuntimeExistsResult, m.RuntimeExistsErr
}

func (m *RuntimeService) Start(_ context.Context) {}

func (m *RuntimeService) Shutdown(_ context.Context) error {
	return nil
}
