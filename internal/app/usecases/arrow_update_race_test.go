package usecases

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// raceFixture wires a catalog advance (PATCH) and an install of the same
// never-installed row over one shared bracket.
type raceFixture struct {
	patch   *patchFixture
	log     *callLog
	stateMu sync.Mutex
	state   domain.ArrowState
	install *runtimeUsecase
	arrowUC *arrowUsecase
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
	f.install = newUC(f.patch.arrow, f.patch.runtime, f.patch.graph)
	f.arrowUC = newArrowUsecase(f.patch.arrow, f.patch.graph, f.patch.runtime, f.install.targets)
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
// waits for the advance, so it assembles the advanced manifest and ref.
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
		_, err := f.arrowUC.Update(context.Background(), rollingRow)
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
	assert.Equal(t, []string{"advance", "begin install"}, f.log.all())
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
		_, err := f.arrowUC.Update(context.Background(), rollingRow)
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
	assert.Equal(t, []string{"begin install"}, f.log.all())
}

// A PATCH whose caller gives up while an install holds the row advances
// nothing.
func TestArrowUpdate_WaitingForTheRow_CallerCanGiveUp(t *testing.T) {
	f := newRaceFixture()
	release, err := f.install.targets.open(context.Background(), rollingRow)
	require.NoError(t, err)
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	patchErr := make(chan error, 1)
	go func() {
		_, err := f.arrowUC.Update(ctx, rollingRow)
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
	release, err := f.install.targets.open(context.Background(), rollingRow)
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
