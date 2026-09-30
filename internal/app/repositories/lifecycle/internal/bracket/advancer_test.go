package bracket

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// patchFixture answers an ArrowUsecase.Update for rollingRow and records
// every advance.
type patchFixture struct {
	arrow    *mocks.MockArrow
	runtime  *mocks.MockRuntime
	graph    *mocks.MockGraph
	advanced []domain.Available
}

func newPatchFixture(state domain.ArrowState, available *domain.Available) *patchFixture {
	f := &patchFixture{}
	f.arrow = &mocks.MockArrow{
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
	f.runtime = &mocks.MockRuntime{
		GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
			return state, nil
		},
	}
	f.graph = &mocks.MockGraph{}
	return f
}

func (f *patchFixture) update(ns domain.Namespace) (models.UpdateResult, error) {
	return NewAdvancer(f.arrow, f.runtime, f.graph, NewTargets()).Recheck(context.Background(), ns)
}

func TestArrowUpdate_NotInstalled_AdvancesTheCatalogRow(t *testing.T) {
	states := []domain.ArrowState{"", domain.ArrowStateAbsent, domain.ArrowStateRemoved}
	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			target := rollingTarget()
			added := domain.Namespace("github.com/user/tool@stable")
			f := newPatchFixture(state, &target)
			f.graph.DiffDepsFn = func(_, _ *domain.Arrow) models.DepDiff {
				return models.DepDiff{Added: []domain.DependencyEdge{{Namespace: added}}}
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

// raceFixture wires a catalog advance (PATCH) and an install of the same
// never-installed row over one shared bracket.
type raceFixture struct {
	patch   *patchFixture
	log     *callLog
	stateMu sync.Mutex
	state   domain.ArrowState
	install *harness
	arrowUC Advancer
	waiting chan struct{}
}

func newRaceFixture() *raceFixture {
	target := rollingTarget()
	f := &raceFixture{log: &callLog{}, state: domain.ArrowStateAbsent, waiting: make(chan struct{})}
	f.patch = newPatchFixture(domain.ArrowStateAbsent, &target)
	f.patch.arrow.ExistsFn = func(context.Context, domain.Namespace) (bool, error) { return true, nil }
	f.patch.arrow.AdvanceFn = func(context.Context, domain.Namespace, domain.Available) error {
		f.log.add("advance")
		return nil
	}
	f.patch.runtime.GetStateFn = func(context.Context, domain.Namespace) (domain.ArrowState, error) {
		f.stateMu.Lock()
		defer f.stateMu.Unlock()
		return f.state, nil
	}
	f.patch.runtime.BeginInstallFn = func(context.Context, domain.Namespace, map[string]string) error {
		f.log.add("begin install")
		f.setState(domain.ArrowStateInstalling)
		return nil
	}
	f.patch.graph.ResolveFn = func(context.Context, domain.Namespace) (models.Plan, error) {
		f.log.add("plan")
		return nil, nil
	}
	f.install = newUC(f.patch.arrow, f.patch.runtime, f.patch.graph)
	f.arrowUC = NewAdvancer(f.patch.arrow, f.patch.runtime, f.patch.graph, f.install.targets)
	var once sync.Once
	f.install.targets.onWait = func(domain.Namespace) { once.Do(func() { close(f.waiting) }) }
	return f
}

func (f *raceFixture) setState(state domain.ArrowState) {
	f.stateMu.Lock()
	defer f.stateMu.Unlock()
	f.state = state
}

// block returns a wait whose first call signals entered and holds until
// release is called.
func block(entered chan<- struct{}) (wait, release func()) {
	unblock := make(chan struct{})
	var once sync.Once
	wait = func() {
		first := false
		once.Do(func() { first = true })
		if first {
			close(entered)
			<-unblock
		}
	}
	return wait, func() { close(unblock) }
}

// An install that starts while PATCH is advancing a never-installed row
// waits for the advance, so it plans the dependencies of the advanced
// manifest and assembles it with the advanced ref.
func TestArrowUpdate_InstallDuringAdvance_WaitsForTheAdvance(t *testing.T) {
	f := newRaceFixture()
	entered := make(chan struct{})
	wait, release := block(entered)
	advance := f.patch.arrow.AdvanceFn
	f.patch.arrow.AdvanceFn = func(ctx context.Context, ns domain.Namespace, target domain.Available) error {
		wait()
		return advance(ctx, ns, target)
	}

	patchErr := make(chan error, 1)
	go func() {
		_, err := f.arrowUC.Recheck(context.Background(), rollingRow)
		patchErr <- err
	}()
	<-entered
	installErr := make(chan error, 1)
	go func() {
		_, err := f.install.Install(context.Background(), rollingRow, nil)
		installErr <- err
	}()
	select {
	case <-f.waiting:
	case err := <-installErr:
		installErr <- err
	}
	release()

	require.NoError(t, <-patchErr)
	require.NoError(t, <-installErr)
	assert.Equal(t, []string{"advance", "plan", "begin install"}, f.log.all())
}

// PATCH that reaches a row while its install is being assembled waits for
// it, then finds the row installed and advances nothing.
func TestArrowUpdate_AdvanceDuringInstall_AdvancesNothing(t *testing.T) {
	f := newRaceFixture()
	entered := make(chan struct{})
	wait, release := block(entered)
	f.patch.runtime.BeginInstallFn = func(context.Context, domain.Namespace, map[string]string) error {
		wait()
		f.log.add("begin install")
		f.setState(domain.ArrowStateInstalling)
		return nil
	}

	installErr := make(chan error, 1)
	go func() {
		_, err := f.install.Install(context.Background(), rollingRow, nil)
		installErr <- err
	}()
	<-entered
	patchErr := make(chan error, 1)
	go func() {
		_, err := f.arrowUC.Recheck(context.Background(), rollingRow)
		patchErr <- err
	}()
	select {
	case <-f.waiting:
	case err := <-patchErr:
		patchErr <- err
	}
	release()

	require.NoError(t, <-installErr)
	require.NoError(t, <-patchErr)
	assert.Equal(t, []string{"plan", "begin install"}, f.log.all())
}

// A PATCH whose caller gives up while an install holds the row advances
// nothing.
func TestArrowUpdate_WaitingForTheRow_CallerCanGiveUp(t *testing.T) {
	f := newRaceFixture()
	release, err := f.install.targets.Open(context.Background(), rollingRow)
	require.NoError(t, err)
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	patchErr := make(chan error, 1)
	go func() {
		_, err := f.arrowUC.Recheck(ctx, rollingRow)
		patchErr <- err
	}()
	<-f.waiting
	cancel()

	require.ErrorIs(t, <-patchErr, context.Canceled)
	assert.Empty(t, f.log.all())
}

// An install whose caller gives up while the row is held begins nothing.
func TestRuntimeInstall_WaitingForTheRow_CallerCanGiveUp(t *testing.T) {
	f := newRaceFixture()
	release, err := f.install.targets.Open(context.Background(), rollingRow)
	require.NoError(t, err)
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	installErr := make(chan error, 1)
	go func() {
		_, err := f.install.Install(ctx, rollingRow, nil)
		installErr <- err
	}()
	<-f.waiting
	cancel()

	require.ErrorIs(t, <-installErr, context.Canceled)
	assert.Empty(t, f.log.all())
}

// An install that waited on the row finds it installed meanwhile and begins
// nothing.
func TestRuntimeInstall_RowInstalledWhileWaiting_BeginsNothing(t *testing.T) {
	f := newRaceFixture()
	release, err := f.install.targets.Open(context.Background(), rollingRow)
	require.NoError(t, err)

	installErr := make(chan error, 1)
	began := make(chan bool, 1)
	go func() {
		ok, err := f.install.Install(context.Background(), rollingRow, nil)
		began <- ok
		installErr <- err
	}()
	<-f.waiting
	f.setState(domain.ArrowStateReady)
	release()

	assert.False(t, <-began)
	require.NoError(t, <-installErr)
	assert.Equal(t, []string{"plan"}, f.log.all())
}

// A state read that fails under the row's bracket begins nothing.
func TestRuntimeInstall_StateReadFailsUnderTheBracket(t *testing.T) {
	f := newRaceFixture()
	reads := 0
	f.patch.runtime.GetStateFn = func(context.Context, domain.Namespace) (domain.ArrowState, error) {
		reads++
		if reads > 1 {
			return "", assert.AnError
		}
		return domain.ArrowStateAbsent, nil
	}

	_, err := f.install.Install(context.Background(), rollingRow, nil)

	require.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, []string{"plan"}, f.log.all())
}

// A dependency install whose caller gives up while another holds the
// dependency's row fails the install.
func TestRuntimeInstall_DependencyRowHeld_CallerCanGiveUp(t *testing.T) {
	f := newRaceFixture()
	dep := domain.Namespace("github.com/user/dep@stable")
	f.patch.graph.ResolveFn = func(context.Context, domain.Namespace) (models.Plan, error) {
		return models.Plan{{Namespace: dep, Type: domain.ToolDep}}, nil
	}
	f.patch.runtime.ListenEndedFn = func(context.Context, domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
		return make(chan domainRuntime.ArrowRuntime), func() {}, nil
	}
	release, err := f.install.targets.Open(context.Background(), dep)
	require.NoError(t, err)
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	installErr := make(chan error, 1)
	go func() {
		_, err := f.install.Install(ctx, rollingRow, nil)
		installErr <- err
	}()
	<-f.waiting
	cancel()

	require.ErrorIs(t, <-installErr, context.Canceled)
	assert.Empty(t, f.log.all())
}
