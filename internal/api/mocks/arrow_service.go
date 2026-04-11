package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type ArrowService struct {
	AddErr              error
	UpdateErr           error
	RemoveErr           error
	ListResult          []arrow.ArrowListDTO
	ListErr             error
	GetResult           *domain.Arrow
	GetErr              error
	GetDetailResult     *arrow.ArrowDetailDTO
	GetDetailErr        error
	HasDependentsResult bool
	HasDependentsErr    error
	InstallErr          error
	UninstallErr        error
	BeginExecutionErr   error
	StopErr             error
}

func (m *ArrowService) Add(_ context.Context, _ domain.Namespace) error {
	return m.AddErr
}

func (m *ArrowService) Update(_ context.Context, _ domain.Namespace) error {
	return m.UpdateErr
}

func (m *ArrowService) Remove(_ context.Context, _ domain.Namespace) error {
	return m.RemoveErr
}

func (m *ArrowService) List(_ context.Context) ([]arrow.ArrowListDTO, error) {
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
