package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type RuntimeService struct {
	InstallStarted      bool
	InstallErr          error
	UninstallErr        error
	ExecuteErr          error
	StopErr             error
	ResetErr            error
	RuntimeExistsResult bool
	RuntimeExistsErr    error
	GetRuntimeResult    *domainRuntime.ArrowRuntime
	GetRuntimeErr       error
	ListRuntimesResult  []domainRuntime.ArrowRuntime
	ListRuntimesErr     error
}

func (m *RuntimeService) GetRuntime(
	_ context.Context,
	_ domain.Namespace,
) (*domainRuntime.ArrowRuntime, error) {
	return m.GetRuntimeResult, m.GetRuntimeErr
}

func (m *RuntimeService) ListRuntimes(
	_ context.Context,
) ([]domainRuntime.ArrowRuntime, error) {
	return m.ListRuntimesResult, m.ListRuntimesErr
}

func (m *RuntimeService) Install(
	_ context.Context,
	_ domain.Namespace,
	_ map[string]string,
) (bool, error) {
	return m.InstallStarted, m.InstallErr
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

func (m *RuntimeService) Reset(
	_ context.Context,
	_ domain.Namespace,
) error {
	return m.ResetErr
}

func (m *RuntimeService) RuntimeExists(
	_ context.Context,
	_ domain.Namespace,
) (bool, error) {
	return m.RuntimeExistsResult, m.RuntimeExistsErr
}

func (m *RuntimeService) Start(_ context.Context) {}
