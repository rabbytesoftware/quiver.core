package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// patchFixture answers an ArrowUsecase.Update for rollingRow and records
// every advance.
type patchFixture struct {
	arrow    *ucmocks.MockArrow
	runtime  *ucmocks.MockRuntime
	graph    *ucmocks.MockGraph
	advanced []domain.Available
}

func newPatchFixture(state domain.ArrowState, available *domain.Available) *patchFixture {
	f := &patchFixture{}
	f.arrow = &ucmocks.MockArrow{
		ResolveCataloguedFn: func(_ context.Context, ns domain.Namespace) (domain.Namespace, error) {
			return ns, nil
		},
		GetFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		CheckAvailableFn: func(context.Context, domain.Namespace) (*domain.Available, error) {
			return available, nil
		},
		AdvanceFn: func(_ context.Context, _ domain.Namespace, target domain.Available) error {
			f.advanced = append(f.advanced, target)
			return nil
		},
	}
	f.runtime = &ucmocks.MockRuntime{
		GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
			return state, nil
		},
	}
	f.graph = &ucmocks.MockGraph{}
	return f
}

func (f *patchFixture) update(ns domain.Namespace) (models.UpdateResult, error) {
	return NewArrowUsecase(f.arrow, f.graph, f.runtime).Update(context.Background(), ns)
}

func TestArrowUpdate_NotInstalled_AdvancesTheCatalogRow(t *testing.T) {
	states := []domain.ArrowState{"", domain.ArrowStateAbsent, domain.ArrowStateRemoved}
	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			target := rollingTarget()
			added := domain.Namespace("github.com/user/tool@stable")
			f := newPatchFixture(state, &target)
			f.graph.DiffDepsFn = func(_, _ *domain.Arrow) graph.DepDiff {
				return graph.DepDiff{Added: []domain.DependencyEdge{{Namespace: added}}}
			}

			result, err := f.update(rollingRow)

			require.NoError(t, err)
			assert.Equal(t, []domain.Available{target}, f.advanced)
			assert.Equal(t, []domain.Namespace{added}, result.AddedDeps)
			assert.Nil(t, result.Available, "an advanced row has nothing left ahead of it")
		})
	}
}

// Only POST /runtime/:ns/update runs the update steps that move an installed
// row, so PATCH reports what is ahead and moves nothing.
func TestArrowUpdate_Installed_ReportsAvailableWithoutMoving(t *testing.T) {
	states := []domain.ArrowState{
		domain.ArrowStateReady,
		domain.ArrowStateOutdated,
		domain.ArrowStateRunning,
		domain.ArrowStateInstalling,
	}
	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			target := rollingTarget()
			f := newPatchFixture(state, &target)

			result, err := f.update(rollingRow)

			require.NoError(t, err)
			assert.Empty(t, f.advanced)
			assert.Equal(t, &target, result.Available)
		})
	}
}

func TestArrowUpdate_CurrentRow_IsANoOp(t *testing.T) {
	f := newPatchFixture(domain.ArrowStateAbsent, nil)
	f.runtime.GetStateFn = func(context.Context, domain.Namespace) (domain.ArrowState, error) {
		t.Error("a current row needs no state")
		return "", nil
	}

	result, err := f.update(rollingRow)

	require.NoError(t, err)
	assert.Empty(t, f.advanced)
	assert.Equal(t, models.UpdateResult{}, result)
}

func TestArrowUpdate_BareNamespaceResolvesToCataloguedRow(t *testing.T) {
	target := rollingTarget()
	f := newPatchFixture(domain.ArrowStateAbsent, &target)
	var advancedOn domain.Namespace
	f.arrow.ResolveCataloguedFn = func(context.Context, domain.Namespace) (domain.Namespace, error) {
		return rollingRow, nil
	}
	f.arrow.AdvanceFn = func(_ context.Context, ns domain.Namespace, _ domain.Available) error {
		advancedOn = ns
		return nil
	}

	_, err := f.update(rollingRow.BareNamespace())

	require.NoError(t, err)
	assert.Equal(t, rollingRow, advancedOn)
}

func TestArrowUpdate_Failures(t *testing.T) {
	boom := errors.New("boom")

	testCases := []struct {
		name    string
		arrange func(f *patchFixture)
	}{
		{
			name: "not catalogued",
			arrange: func(f *patchFixture) {
				f.arrow.ResolveCataloguedFn = func(context.Context, domain.Namespace) (domain.Namespace, error) {
					return "", boom
				}
			},
		},
		{
			name: "row cannot be read",
			arrange: func(f *patchFixture) {
				f.arrow.GetFn = func(context.Context, domain.Namespace) (*domain.Arrow, error) { return nil, boom }
			},
		},
		{
			name: "re-resolve fails",
			arrange: func(f *patchFixture) {
				f.arrow.CheckAvailableFn = func(context.Context, domain.Namespace) (*domain.Available, error) {
					return nil, boom
				}
			},
		},
		{
			name: "state cannot be read",
			arrange: func(f *patchFixture) {
				f.runtime.GetStateFn = func(context.Context, domain.Namespace) (domain.ArrowState, error) {
					return "", boom
				}
			},
		},
		{
			name: "advance fails",
			arrange: func(f *patchFixture) {
				f.arrow.AdvanceFn = func(context.Context, domain.Namespace, domain.Available) error { return boom }
			},
		},
		{
			name: "advanced row cannot be read back",
			arrange: func(f *patchFixture) {
				reads := 0
				f.arrow.GetFn = func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
					reads++
					if reads > 1 {
						return nil, boom
					}
					return &domain.Arrow{Namespace: ns}, nil
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target := rollingTarget()
			f := newPatchFixture(domain.ArrowStateAbsent, &target)
			tc.arrange(f)

			_, err := f.update(rollingRow)

			require.ErrorIs(t, err, boom)
		})
	}
}
