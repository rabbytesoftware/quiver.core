package usecases

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/deptree"
)

const (
	cycleX = domain.Namespace("github.com/user/x@stable")
	cycleY = domain.Namespace("github.com/user/y@stable")
)

// depRows is an in-memory runtime for rows whose updates gain dependencies.
type depRows struct {
	mu       sync.Mutex
	state    map[domain.Namespace]domain.ArrowState
	pending  map[domain.Namespace]*domainRuntime.DepSyncInfo
	installs []domain.Namespace
	updates  []domain.Namespace
}

func newDepRows(rows ...domain.Namespace) *depRows {
	d := &depRows{
		state:   make(map[domain.Namespace]domain.ArrowState),
		pending: make(map[domain.Namespace]*domainRuntime.DepSyncInfo),
	}
	for _, ns := range rows {
		d.state[ns] = domain.ArrowStateReady
	}
	return d
}

func (d *depRows) runtime() *ucmocks.MockRuntime {
	return &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			d.mu.Lock()
			defer d.mu.Unlock()
			return d.state[ns], nil
		},
		GetRuntimeFn: func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			d.mu.Lock()
			defer d.mu.Unlock()
			return &domainRuntime.ArrowRuntime{Ref: ns, State: d.state[ns], PendingDepSync: d.pending[ns]}, nil
		},
		MarkOutdatedFn: func(_ context.Context, ns domain.Namespace, added, removed []domain.Namespace) error {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.state[ns] = domain.ArrowStateOutdated
			d.pending[ns] = &domainRuntime.DepSyncInfo{AddedDeps: added, RemovedDeps: removed}
			return nil
		},
		ListenEndedFn: func(context.Context, domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			ch := make(chan domainRuntime.ArrowRuntime, 1)
			ch <- domainRuntime.ArrowRuntime{}
			return ch, func() {}, nil
		},
		BeginInstallFn: func(_ context.Context, ns domain.Namespace, _ map[string]string) error {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.installs = append(d.installs, ns)
			return nil
		},
		BeginUpdateFn: func(_ context.Context, ns domain.Namespace, _ map[string]string, _ string) error {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.updates = append(d.updates, ns)
			d.state[ns] = domain.ArrowStateUpdating
			return nil
		},
	}
}

// depArrows answers the catalog side of an update whose target adds the
// dependency adds names for the row.
func depArrows(adds map[domain.Namespace]domain.Namespace, staged func(domain.Namespace)) (*ucmocks.MockArrow, *ucmocks.MockGraph) {
	target := rollingTarget()
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return true, nil },
		CheckAvailableFn: func(context.Context, domain.Namespace) (*domain.Available, error) {
			return &target, nil
		},
		RefreshToTargetFn: func(_ context.Context, ns domain.Namespace, _ domain.Available) (*domain.Arrow, error) {
			staged(ns)
			return &domain.Arrow{Namespace: ns}, nil
		},
		AddDependencyFn: func(_ context.Context, declared domain.Namespace) (domain.Namespace, error) {
			return declared.WithRef("stable"), nil
		},
	}
	g := &ucmocks.MockGraph{
		DiffDepsFn: func(_, next *domain.Arrow) graph.DepDiff {
			return graph.DepDiff{Added: []domain.DependencyEdge{{Namespace: adds[next.Namespace]}}}
		},
	}
	return a, g
}

// failOnBracketWait makes any wait on a row's bracket fail the test and end
// every caller: an update installing its dependencies must never wait on a
// bracket, or two of them can wait on each other forever.
func failOnBracketWait(t *testing.T, uc *runtimeUsecase, cancel context.CancelFunc) {
	t.Helper()
	uc.targets.onWait = func(ns domain.Namespace) {
		t.Errorf("an update waited on the bracket of %s", ns)
		cancel()
	}
}

// A target that adds a dependency on the row being updated is refused
// before anything is installed, instead of waiting on the row's own bracket.
func TestRuntimeUpdate_TargetDependsOnItsOwnRow_IsRefused(t *testing.T) {
	rows := newDepRows(cycleX)
	a, g := depArrows(map[domain.Namespace]domain.Namespace{cycleX: cycleX.BareNamespace()}, func(domain.Namespace) {})
	uc := newUC(a, rows.runtime(), g)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failOnBracketWait(t, uc, cancel)

	_, err := uc.Update(ctx, cycleX, nil)

	require.ErrorIs(t, err, apperrors.ErrInvalidManifest)
	assert.Empty(t, rows.installs)
	assert.Empty(t, rows.updates)
}

// Two rows whose targets add each other update concurrently. Neither waits on
// the other's bracket; a cycle the graph already sees refuses both, and one
// it does not see yet lets both begin.
func TestRuntimeUpdate_TargetsAddEachOther_NeverWaitOnEachOther(t *testing.T) {
	testCases := []struct {
		name        string
		resolveErr  error
		wantErr     error
		wantUpdates int
	}{
		{name: "the graph sees the cycle", resolveErr: &deptree.CycleError{Path: []domain.Namespace{cycleX, cycleY, cycleX}}, wantErr: apperrors.ErrInvalidManifest},
		{name: "the graph does not see it yet", wantUpdates: 2},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rows := newDepRows(cycleX, cycleY)
			bothStaged := make(chan struct{})
			var stagedOnce sync.WaitGroup
			stagedOnce.Add(2)
			go func() { stagedOnce.Wait(); close(bothStaged) }()
			a, g := depArrows(map[domain.Namespace]domain.Namespace{cycleX: cycleY, cycleY: cycleX}, func(domain.Namespace) {
				stagedOnce.Done()
				<-bothStaged
			})
			g.ResolveFn = func(context.Context, domain.Namespace) (graph.Plan, error) {
				return nil, tc.resolveErr
			}
			uc := newUC(a, rows.runtime(), g)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failOnBracketWait(t, uc, cancel)

			errs := make(chan error, 2)
			for _, ns := range []domain.Namespace{cycleX, cycleY} {
				go func() {
					_, err := uc.Update(ctx, ns, nil)
					errs <- err
				}()
			}

			for range 2 {
				err := <-errs
				if tc.wantErr == nil {
					require.NoError(t, err)
					continue
				}
				require.ErrorIs(t, err, tc.wantErr)
			}
			assert.Len(t, rows.updates, tc.wantUpdates)
			assert.Empty(t, rows.installs, fmt.Sprintf("installed rows are never installed again: %v", rows.installs))
		})
	}
}
