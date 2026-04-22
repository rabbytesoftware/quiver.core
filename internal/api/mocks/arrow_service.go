package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type ArrowService struct {
	AddErr                 error
	UpdateResult           arrow.UpdateResult
	UpdateErr              error
	RemoveErr              error
	ListResult             []arrow.ArrowListDTO
	ListErr                error
	ListUserInstalledArg   *bool
	GetResult              *domain.Arrow
	GetErr                 error
	GetDetailResult        *arrow.ArrowDetailDTO
	GetDetailErr           error
	GetManifestResult      *arrow.ArrowManifestDTO
	GetManifestErr         error
	HasDependentsResult    bool
	HasDependentsErr       error
	InstallErr             error
	UninstallErr           error
	BeginExecutionErr      error
	StopErr                error
	SeedErr                error
	ValidateManifestResult *arrow.ValidationResult
	ValidateManifestErr    error
}

func (m *ArrowService) Add(_ context.Context, _ domain.Namespace) error {
	return m.AddErr
}

func (m *ArrowService) Update(
	_ context.Context,
	_ domain.Namespace,
	_ arrow.UpdateOptions,
) (arrow.UpdateResult, error) {
	return m.UpdateResult, m.UpdateErr
}

func (m *ArrowService) Remove(_ context.Context, _ domain.Namespace) error {
	return m.RemoveErr
}

func (m *ArrowService) List(_ context.Context, userInstalled *bool) ([]arrow.ArrowListDTO, error) {
	m.ListUserInstalledArg = userInstalled
	return m.ListResult, m.ListErr
}

func (m *ArrowService) Get(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
	return m.GetResult, m.GetErr
}

func (m *ArrowService) GetDetail(
	_ context.Context,
	_ domain.Namespace,
) (*arrow.ArrowDetailDTO, error) {
	return m.GetDetailResult, m.GetDetailErr
}

func (m *ArrowService) GetManifest(
	_ context.Context,
	_ domain.Namespace,
) (*arrow.ArrowManifestDTO, error) {
	return m.GetManifestResult, m.GetManifestErr
}

func (m *ArrowService) HasDependents(
	_ context.Context,
	_ domain.Namespace,
	_ domain.Namespace,
) (bool, error) {
	return m.HasDependentsResult, m.HasDependentsErr
}

func (m *ArrowService) Install(
	_ context.Context,
	_ domain.Namespace,
	_ map[string]string,
) error {
	return m.InstallErr
}

func (m *ArrowService) Uninstall(
	_ context.Context,
	_ domain.Namespace,
	_ map[string]string,
) error {
	return m.UninstallErr
}

func (m *ArrowService) BeginExecution(
	_ context.Context,
	_ domain.Namespace,
	_ string,
	_ map[string]string,
) error {
	return m.BeginExecutionErr
}

func (m *ArrowService) Stop(_ context.Context, _ domain.Namespace) error {
	return m.StopErr
}

func (m *ArrowService) Seed(_ context.Context, _ domain.Namespace, _ []byte) error {
	return m.SeedErr
}

func (m *ArrowService) ValidateManifest(
	_ context.Context,
	_ domain.Namespace,
	_ []byte,
) (*arrow.ValidationResult, error) {
	return m.ValidateManifestResult, m.ValidateManifestErr
}

func (m *ArrowService) Shutdown(_ context.Context) error {
	return nil
}
