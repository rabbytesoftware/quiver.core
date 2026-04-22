package mocks

import (
	"context"

	catstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type Catalog struct {
	AddErr           error
	UpdateErr        error
	RemoveErr        error
	ListArrowsValue  []catstore.ArrowViewModel
	ListErr          error
	GetArrowValue    *catstore.ArrowViewModel
	GetErr           error
	IsInstalledValue bool
	VersionsValue    []catstore.ArrowViewModel
	VersionsErr      error
}

func (m *Catalog) Add(ctx context.Context, ns domain.Namespace, a *domain.Arrow, ui bool, ref string) error {
	return m.AddErr
}

func (m *Catalog) Update(ctx context.Context, ns domain.Namespace, a *domain.Arrow) error {
	return m.UpdateErr
}

func (m *Catalog) Remove(ctx context.Context, ns domain.Namespace) error {
	return m.RemoveErr
}

func (m *Catalog) Retire(ctx context.Context, ns domain.Namespace) error {
	return nil
}

func (m *Catalog) List(ctx context.Context) ([]catstore.ArrowViewModel, error) {
	return m.ListArrowsValue, m.ListErr
}

func (m *Catalog) Get(ctx context.Context, ns domain.Namespace) (*catstore.ArrowViewModel, error) {
	return m.GetArrowValue, m.GetErr
}

func (m *Catalog) IsInstalled(ctx context.Context, ns domain.Namespace) bool {
	return m.IsInstalledValue
}

func (m *Catalog) ListVersions(ctx context.Context, ns domain.Namespace) ([]catstore.ArrowViewModel, error) {
	return m.VersionsValue, m.VersionsErr
}
