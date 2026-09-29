package usecases

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

const rollingRow = domain.Namespace("github.com/char2cs/crowbar@nightly-latest")

func rollingTarget() domain.Available {
	return domain.Available{Ref: "nightly-latest", Commit: "c2"}
}

// callLog records the order the bracket's steps run in.
type callLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *callLog) add(call string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, call)
}

func (l *callLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

// bracketFixture wires the mocks an update bracket touches, logging each step.
type bracketFixture struct {
	log       *callLog
	arrow     *ucmocks.MockArrow
	runtime   *ucmocks.MockRuntime
	graph     *ucmocks.MockGraph
	state     domain.ArrowState
	available *domain.Available
	stateMu   sync.Mutex
}

func newBracketFixture(state domain.ArrowState, available *domain.Available) *bracketFixture {
	f := &bracketFixture{log: &callLog{}, state: state, available: available}
	f.arrow = &ucmocks.MockArrow{
		GetFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		CheckAvailableFn: func(context.Context, domain.Namespace) (*domain.Available, error) {
			f.log.add("check available")
			return f.available, nil
		},
		RefreshToTargetFn: func(_ context.Context, ns domain.Namespace, target domain.Available) (*domain.Arrow, error) {
			f.log.add("refresh to " + target.Commit)
			return &domain.Arrow{Namespace: ns}, nil
		},
	}
	f.runtime = &ucmocks.MockRuntime{
		GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
			return f.currentState(), nil
		},
		GetRuntimeFn: func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{Ref: ns, State: f.currentState()}, nil
		},
		ListenEndedFn: func(context.Context, domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			ch := make(chan domainRuntime.ArrowRuntime, 1)
			ch <- domainRuntime.ArrowRuntime{}
			return ch, func() {}, nil
		},
		BeginStopFn: func(context.Context, domain.Namespace) error {
			f.log.add("stop")
			f.setState(domain.ArrowStateReady)
			return nil
		},
		BeginUpdateFn: func(context.Context, domain.Namespace, map[string]string) error {
			f.log.add("begin update")
			return nil
		},
	}
	f.graph = &ucmocks.MockGraph{}
	return f
}

func (f *bracketFixture) currentState() domain.ArrowState {
	f.stateMu.Lock()
	defer f.stateMu.Unlock()
	return f.state
}

func (f *bracketFixture) setState(state domain.ArrowState) {
	f.stateMu.Lock()
	defer f.stateMu.Unlock()
	f.state = state
}

func (f *bracketFixture) usecase() *runtimeUsecase {
	return newUC(f.arrow, f.runtime, f.graph)
}

// ─── executeUpdate ───────────────────────────────────────────────────────────

func TestRuntimeExecute_Update_OutdatedRow_RunsTheBracketInOrder(t *testing.T) {
	testCases := []struct {
		name  string
		state domain.ArrowState
		want  []string
	}{
		{"ready", domain.ArrowStateReady, []string{"check available", "refresh to c2", "begin update"}},
		{"outdated", domain.ArrowStateOutdated, []string{"check available", "refresh to c2", "begin update"}},
		{"running is stopped first", domain.ArrowStateRunning, []string{"check available", "stop", "refresh to c2", "begin update"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target := rollingTarget()
			f := newBracketFixture(tc.state, &target)
			var vars map[string]string
			f.runtime.BeginUpdateFn = func(_ context.Context, _ domain.Namespace, got map[string]string) error {
				f.log.add("begin update")
				vars = got
				return nil
			}
			uc := f.usecase()

			err := uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, map[string]string{"TOKEN": "x"})

			require.NoError(t, err)
			assert.Equal(t, tc.want, f.log.all())
			assert.Equal(t, map[string]string{"TOKEN": "x"}, vars)
			recorded, ok := uc.targets.take(rollingRow)
			require.True(t, ok, "the update remembers the target it began toward")
			assert.Equal(t, target, recorded)
		})
	}
}

func TestRuntimeExecute_Update_CurrentRow_DoesNothing(t *testing.T) {
	f := newBracketFixture(domain.ArrowStateRunning, nil)
	uc := f.usecase()

	err := uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"check available"}, f.log.all(), "a current row is neither stopped nor updated")
	_, ok := uc.targets.take(rollingRow)
	assert.False(t, ok)
}

func TestRuntimeExecute_Update_NotIdle_FallsBackToTheMethod(t *testing.T) {
	f := newBracketFixture(domain.ArrowStateAbsent, nil)
	var method string
	f.runtime.BeginExecutionFn = func(_ context.Context, _ domain.Namespace, m string, _ map[string]string) error {
		method = m
		return nil
	}

	err := f.usecase().Execute(context.Background(), rollingRow, domain.MethodUpdate, nil)

	require.NoError(t, err)
	assert.Equal(t, domain.MethodUpdate, method)
	assert.Empty(t, f.log.all(), "only an installed, idle or running row enters the bracket")
}

// A target that gains a dependency lands the row outdated with the new
// dependency pending, and the dependency is installed before the target's
// update steps run.
func TestRuntimeExecute_Update_TargetGainsADependency_InstallsItFirst(t *testing.T) {
	target := rollingTarget()
	dep := domain.Namespace("github.com/user/tool@v1.*")
	f := newBracketFixture(domain.ArrowStateReady, &target)
	var pending *domainRuntime.DepSyncInfo
	f.graph.DiffDepsFn = func(_, _ *domain.Arrow) graph.DepDiff {
		return graph.DepDiff{Added: []domain.DependencyEdge{{Namespace: dep, Constraint: "v1.*"}}}
	}
	f.runtime.MarkOutdatedFn = func(_ context.Context, _ domain.Namespace, added, removed []domain.Namespace) error {
		f.log.add("mark outdated")
		pending = &domainRuntime.DepSyncInfo{AddedDeps: added, RemovedDeps: removed}
		f.setState(domain.ArrowStateOutdated)
		return nil
	}
	f.runtime.GetStateFn = func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
		if ns == dep {
			return domain.ArrowStateAbsent, nil
		}
		return f.currentState(), nil
	}
	f.runtime.GetRuntimeFn = func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
		return &domainRuntime.ArrowRuntime{Ref: ns, State: f.currentState(), PendingDepSync: pending}, nil
	}
	f.runtime.ListenEndedFn = endedWith(domainRuntime.ExecutionOutcomeSuccess)
	f.runtime.BeginInstallFn = func(_ context.Context, ns domain.Namespace, _ map[string]string) error {
		f.log.add("install " + ns.String())
		return nil
	}

	err := f.usecase().Execute(context.Background(), rollingRow, domain.MethodUpdate, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"check available",
		"refresh to c2",
		"mark outdated",
		"install " + dep.String(),
		"begin update",
	}, f.log.all())
	require.NotNil(t, pending)
	assert.Equal(t, []domain.Namespace{dep}, pending.AddedDeps)
}

// A retried update finds the target manifest already staged, so the diff is
// empty; what the first attempt left pending is still synced.
func TestRuntimeExecute_Update_Retry_SyncsWhatIsStillPending(t *testing.T) {
	target := rollingTarget()
	dep := domain.Namespace("github.com/user/tool@stable")
	f := newBracketFixture(domain.ArrowStateOutdated, &target)
	f.runtime.GetStateFn = func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
		if ns == dep {
			return domain.ArrowStateReady, nil
		}
		return domain.ArrowStateOutdated, nil
	}
	f.runtime.GetRuntimeFn = func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
		return &domainRuntime.ArrowRuntime{
			Ref:            ns,
			State:          domain.ArrowStateOutdated,
			PendingDepSync: &domainRuntime.DepSyncInfo{AddedDeps: []domain.Namespace{dep}},
		}, nil
	}
	var ensured []domain.Namespace
	f.arrow.ExistsFn = func(_ context.Context, ns domain.Namespace) (bool, error) {
		ensured = append(ensured, ns)
		return true, nil
	}

	err := f.usecase().Execute(context.Background(), rollingRow, domain.MethodUpdate, nil)

	require.NoError(t, err)
	assert.Equal(t, []domain.Namespace{dep}, ensured)
	assert.Equal(t, []string{"check available", "refresh to c2", "begin update"}, f.log.all())
}

func TestRuntimeExecute_Update_Failures(t *testing.T) {
	boom := errors.New("boom")

	testCases := []struct {
		name     string
		arrange  func(f *bracketFixture)
		wantLog  []string
		wantKept bool
	}{
		{
			name: "current row cannot be read",
			arrange: func(f *bracketFixture) {
				f.arrow.GetFn = func(context.Context, domain.Namespace) (*domain.Arrow, error) { return nil, boom }
			},
		},
		{
			name: "re-resolve fails",
			arrange: func(f *bracketFixture) {
				f.arrow.CheckAvailableFn = func(context.Context, domain.Namespace) (*domain.Available, error) {
					return nil, boom
				}
			},
		},
		{
			name: "stop fails",
			arrange: func(f *bracketFixture) {
				f.setState(domain.ArrowStateRunning)
				f.runtime.BeginStopFn = func(context.Context, domain.Namespace) error { return boom }
			},
			wantLog: []string{"check available"},
		},
		{
			name: "target manifest cannot be staged",
			arrange: func(f *bracketFixture) {
				f.arrow.RefreshToTargetFn = func(context.Context, domain.Namespace, domain.Available) (*domain.Arrow, error) {
					f.log.add("refresh")
					return nil, boom
				}
			},
			wantLog: []string{"check available", "refresh"},
		},
		{
			name: "dependency change cannot be recorded",
			arrange: func(f *bracketFixture) {
				f.graph.DiffDepsFn = func(_, _ *domain.Arrow) graph.DepDiff {
					return graph.DepDiff{Removed: []domain.DependencyEdge{{Namespace: "github.com/user/old@v1"}}}
				}
				f.runtime.MarkOutdatedFn = func(context.Context, domain.Namespace, []domain.Namespace, []domain.Namespace) error {
					return boom
				}
			},
			wantLog: []string{"check available", "refresh to c2"},
		},
		{
			name: "state after staging cannot be read",
			arrange: func(f *bracketFixture) {
				calls := 0
				f.runtime.GetStateFn = func(context.Context, domain.Namespace) (domain.ArrowState, error) {
					calls++
					if calls > 2 {
						return "", boom
					}
					return domain.ArrowStateReady, nil
				}
			},
			wantLog: []string{"check available", "refresh to c2"},
		},
		{
			name: "pending dependencies cannot be synced",
			arrange: func(f *bracketFixture) {
				f.setState(domain.ArrowStateOutdated)
				f.runtime.GetRuntimeFn = func(context.Context, domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
					return nil, boom
				}
			},
			wantLog: []string{"check available", "refresh to c2"},
		},
		{
			name: "update cannot begin",
			arrange: func(f *bracketFixture) {
				f.runtime.BeginUpdateFn = func(context.Context, domain.Namespace, map[string]string) error {
					f.log.add("begin update")
					return boom
				}
			},
			wantLog: []string{"check available", "refresh to c2", "begin update"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target := rollingTarget()
			f := newBracketFixture(domain.ArrowStateReady, &target)
			tc.arrange(f)
			uc := f.usecase()

			err := uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil)

			require.ErrorIs(t, err, boom)
			if tc.wantLog == nil {
				assert.NotContains(t, f.log.all(), "begin update")
			} else {
				assert.Equal(t, tc.wantLog, f.log.all())
			}
			_, kept := uc.targets.take(rollingRow)
			assert.False(t, kept, "an update that never began leaves no target behind")
		})
	}
}

// ─── stopIfRunning ───────────────────────────────────────────────────────────

func TestStopIfRunning(t *testing.T) {
	boom := errors.New("boom")

	testCases := []struct {
		name      string
		state     domain.ArrowState
		stateErr  error
		listenErr error
		stopErr   error
		cancel    bool
		wantStop  bool
		wantErr   error
	}{
		{name: "idle is left alone", state: domain.ArrowStateReady},
		{name: "running is stopped and awaited", state: domain.ArrowStateRunning, wantStop: true},
		{name: "state cannot be read", stateErr: boom, wantErr: boom},
		{name: "cannot listen", state: domain.ArrowStateRunning, listenErr: boom, wantErr: boom},
		{name: "stop is rejected", state: domain.ArrowStateRunning, stopErr: boom, wantStop: true, wantErr: boom},
		{name: "caller gives up", state: domain.ArrowStateRunning, cancel: true, wantStop: true, wantErr: context.Canceled},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stopped := false
			rt := &ucmocks.MockRuntime{
				GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
					return tc.state, tc.stateErr
				},
				ListenEndedFn: func(context.Context, domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
					ch := make(chan domainRuntime.ArrowRuntime, 1)
					if !tc.cancel {
						ch <- domainRuntime.ArrowRuntime{}
					}
					return ch, func() {}, tc.listenErr
				},
				BeginStopFn: func(context.Context, domain.Namespace) error {
					stopped = true
					if tc.cancel {
						cancel()
					}
					return tc.stopErr
				},
			}

			err := stopIfRunning(ctx, rt, rollingRow)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantStop, stopped)
		})
	}
}

// ─── onUpdateEnded ───────────────────────────────────────────────────────────

func updateEnded(ns domain.Namespace, outcome domainRuntime.ExecutionOutcome) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:        ns,
		LastReturn: &domainRuntime.Return{Method: domain.MethodUpdate, Outcome: outcome},
	}
}

// commitFixture logs every step onUpdateEnded may take.
func commitFixture(unmoved bool, unmovedErr error) (*ucmocks.MockArrow, *ucmocks.MockRuntime, *callLog) {
	log := &callLog{}
	a := &ucmocks.MockArrow{
		TargetUnmovedFn: func(_ context.Context, _ domain.Namespace, target domain.Available) (bool, error) {
			log.add("re-resolve " + target.Commit)
			return unmoved, unmovedErr
		},
		AdvanceFn: func(_ context.Context, _ domain.Namespace, target domain.Available) error {
			log.add("advance " + target.Commit)
			return nil
		},
	}
	rt := &ucmocks.MockRuntime{
		ClearVersionBadgeFn: func(context.Context, domain.Namespace) error {
			log.add("clear badge")
			return nil
		},
	}
	return a, rt, log
}

func TestRuntimeOnUpdateEnded_TargetUnchanged_AdvancesThenClearsTheBadge(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	uc.targets.put(rollingRow, rollingTarget())

	uc.onRuntimeEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Equal(t, []string{"re-resolve c2", "advance c2", "clear badge"}, log.all())
}

func TestRuntimeOnUpdateEnded_StampsNothing(t *testing.T) {
	testCases := []struct {
		name       string
		rt         domainRuntime.ArrowRuntime
		record     bool
		unmoved    bool
		unmovedErr error
		wantLog    []string
	}{
		{
			name:    "target moved during the update",
			rt:      updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess),
			record:  true,
			wantLog: []string{"re-resolve c2"},
		},
		{
			name:       "target cannot be re-resolved",
			rt:         updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess),
			record:     true,
			unmovedErr: errors.New("remote down"),
			wantLog:    []string{"re-resolve c2"},
		},
		{
			name:    "update steps failed",
			rt:      updateEnded(rollingRow, domainRuntime.ExecutionOutcomeFailed),
			record:  true,
			unmoved: true,
		},
		{
			name:    "no return recorded",
			rt:      domainRuntime.ArrowRuntime{Ref: rollingRow},
			record:  true,
			unmoved: true,
		},
		{
			name:    "no update began toward a target",
			rt:      updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess),
			unmoved: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a, rt, log := commitFixture(tc.unmoved, tc.unmovedErr)
			uc := newUC(a, rt, &ucmocks.MockGraph{})
			if tc.record {
				uc.targets.put(rollingRow, rollingTarget())
			}

			uc.onUpdateEnded(context.Background(), tc.rt)

			assert.Equal(t, tc.wantLog, log.all())
		})
	}
}

// A failed attempt keeps its target, so a retried update that succeeds still
// commits it.
func TestRuntimeOnUpdateEnded_FailedAttemptKeepsTheTarget(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	uc.targets.put(rollingRow, rollingTarget())

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeFailed))
	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Equal(t, []string{"re-resolve c2", "advance c2", "clear badge"}, log.all())
}

// A version check during the update may record a newer target on the row;
// the commit stamps the target the update actually ran.
func TestRuntimeOnUpdateEnded_CommitsTheTargetTheUpdateRan(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	a.GetFn = func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		return &domain.Arrow{Namespace: ns, Available: &domain.Available{Ref: "nightly-latest", Commit: "c3"}}, nil
	}
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	uc.targets.put(rollingRow, rollingTarget())

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Equal(t, []string{"re-resolve c2", "advance c2", "clear badge"}, log.all())
}

func TestRuntimeOnUpdateEnded_AdvanceFails_BadgeStays(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	a.AdvanceFn = func(context.Context, domain.Namespace, domain.Available) error {
		log.add("advance failed")
		return errors.New("fetch failed")
	}
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	uc.targets.put(rollingRow, rollingTarget())

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Equal(t, []string{"re-resolve c2", "advance failed"}, log.all())
}

func TestRuntimeOnUpdateEnded_ClearBadgeFails_IsOnlyLogged(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	rt.ClearVersionBadgeFn = func(context.Context, domain.Namespace) error {
		log.add("clear badge failed")
		return errors.New("event store down")
	}
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	uc.targets.put(rollingRow, rollingTarget())

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Equal(t, []string{"re-resolve c2", "advance c2", "clear badge failed"}, log.all())
}

// quiver.core's relaunched binary adopts its own new state; its row is not
// advanced from here.
func TestRuntimeOnUpdateEnded_SelfNamespace_Skipped(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	selfRow := self.WithRef("stable")
	a, rt, log := commitFixture(true, nil)
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	uc.targets.put(selfRow, rollingTarget())

	uc.onUpdateEnded(context.Background(), updateEnded(selfRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Empty(t, log.all())
}

// The commit leaves the runtime aggregate's ordered delivery before it
// clears that aggregate's badge, which would otherwise wait for itself.
func TestRuntimeOnUpdateEnded_CommitsOffTheDeliveringGoroutine(t *testing.T) {
	a, rt, _ := commitFixture(true, nil)
	cleared := make(chan struct{})
	rt.ClearVersionBadgeFn = func(context.Context, domain.Namespace) error {
		close(cleared)
		return nil
	}
	uc := newRuntimeUsecase(a, rt, &ucmocks.MockGraph{})
	uc.targets.put(rollingRow, rollingTarget())

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	select {
	case <-cleared:
	case <-time.After(5 * time.Second):
		t.Fatal("the detached commit never cleared the badge")
	}
}
