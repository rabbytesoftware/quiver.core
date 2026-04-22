package mocks

import (
	"context"

	appDeps "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type Deps struct {
	ResolveErr       error
	ResolvePlanValue appDeps.Plan
	ExecuteErr       error
	HasDepValue      bool
	HasDepErr        error
	OrphansValue     []domain.Namespace
	OrphansErr       error
	DiffResultValue  appDeps.DepDiff
}

func (m *Deps) Resolve(ctx context.Context, ns domain.Namespace) (appDeps.Plan, error) {
	return m.ResolvePlanValue, m.ResolveErr
}

func (m *Deps) Execute(ctx context.Context, p appDeps.Plan, ns domain.Namespace) error {
	return m.ExecuteErr
}

func (m *Deps) Unplan(ctx context.Context, ns domain.Namespace) (appDeps.Plan, error) {
	return nil, nil
}

func (m *Deps) HasDependents(ctx context.Context, ns domain.Namespace, exclude domain.Namespace) (bool, error) {
	return m.HasDepValue, m.HasDepErr
}

func (m *Deps) Orphans(ctx context.Context, ns domain.Namespace) ([]domain.Namespace, error) {
	return m.OrphansValue, m.OrphansErr
}

func (m *Deps) DiffDeps(old *domain.Arrow, new *domain.Arrow) appDeps.DepDiff {
	return m.DiffResultValue
}
