package deps

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// refusedBrackets stands in for a row whose bracket the caller gave up
// waiting on.
type refusedBrackets struct {
	err error
}

func (b refusedBrackets) Open(context.Context, domain.Namespace) (func(), error) {
	return nil, b.err
}

func TestRuntimeInstall_BracketRefused_BeginsNothing(t *testing.T) {
	boom := errors.New("boom")
	dep := domain.Namespace("github.com/user/dep@stable")

	testCases := []struct {
		name string
		plan models.Plan
	}{
		{name: "the row's own bracket"},
		{name: "a dependency's bracket", plan: models.Plan{{Namespace: dep, Type: domain.ToolDep}}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			began := false
			a := &mocks.MockArrow{ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return true, nil }}
			rt := &mocks.MockRuntime{
				BeginInstallFn: func(context.Context, domain.Namespace, map[string]string) error {
					began = true
					return nil
				},
			}
			g := &mocks.MockGraph{ResolveFn: func(context.Context, domain.Namespace) (models.Plan, error) { return tc.plan, nil }}
			uc := newUC(a, rt, g)
			uc.deps.brackets = refusedBrackets{err: boom}

			_, err := uc.Install(context.Background(), rollingRow, nil)

			require.ErrorIs(t, err, boom)
			assert.False(t, began)
		})
	}
}

// A dependency read again under its bracket may already be installing, or
// unreadable: either way nothing begins.
func TestInstallOneDep_StateUnderTheBracket(t *testing.T) {
	boom := errors.New("boom")

	testCases := []struct {
		name     string
		second   domain.ArrowState
		stateErr error
		wantErr  error
	}{
		{name: "installed meanwhile", second: domain.ArrowStateReady},
		{name: "unreadable", stateErr: boom, wantErr: boom},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reads := 0
			began := false
			rt := &mocks.MockRuntime{
				GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
					reads++
					if reads == 1 {
						return domain.ArrowStateAbsent, nil
					}
					return tc.second, tc.stateErr
				},
				ListenEndedFn: func(context.Context, domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
					ch := make(chan domainRuntime.ArrowRuntime, 1)
					ch <- domainRuntime.ArrowRuntime{}
					return ch, func() {}, nil
				},
				BeginInstallFn: func(context.Context, domain.Namespace, map[string]string) error {
					began = true
					return nil
				},
			}

			err := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{}).installOneDep(context.Background(), "github.com/user/dep@stable")

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.False(t, began)
		})
	}
}

func TestRuntimeVerbs_UnresolvedNamespace_BeginNothing(t *testing.T) {
	a := &mocks.MockArrow{
		ResolveCataloguedFn: func(context.Context, domain.Namespace) (domain.Namespace, error) {
			return "", assert.AnError
		},
	}
	began := false
	rt := &mocks.MockRuntime{
		BeginStopFn: func(context.Context, domain.Namespace) error {
			began = true
			return nil
		},
		BeginUninstallFn: func(context.Context, domain.Namespace, map[string]string) error {
			began = true
			return nil
		},
	}
	uc := newUC(a, rt, &mocks.MockGraph{})

	require.ErrorIs(t, uc.Stop(context.Background(), "github.com/user/nope"), assert.AnError)
	require.ErrorIs(t, uc.Uninstall(context.Background(), "github.com/user/nope", nil), assert.AnError)
	assert.False(t, began)
}

func TestRuntimeOnEnded_Update_HandsOverToTheSettling(t *testing.T) {
	uc := newUC(&mocks.MockArrow{}, &mocks.MockRuntime{}, &mocks.MockGraph{})
	ended := domainRuntime.ArrowRuntime{
		Ref:        rollingRow,
		LastReturn: &domainRuntime.Return{Method: domain.MethodUpdate, Outcome: domainRuntime.ExecutionOutcomeSuccess},
	}

	uc.onRuntimeEnded(context.Background(), ended)

	assert.Equal(t, []domainRuntime.ArrowRuntime{ended}, uc.updatesEnded)
}
