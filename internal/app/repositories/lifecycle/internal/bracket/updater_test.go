package bracket

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/deptree"
)

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
			f.runtime.BeginUpdateFn = func(_ context.Context, _ domain.Namespace, got map[string]string, _ string) error {
				f.log.add("begin update")
				vars = got
				return nil
			}
			uc := f.usecase()

			err := uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, map[string]string{"TOKEN": "x"})

			require.NoError(t, err)
			assert.Equal(t, tc.want, f.log.all())
			assert.Equal(t, map[string]string{"TOKEN": "x"}, vars)
			recorded, ok := uc.targets.Take(rollingRow)
			require.True(t, ok, "the update remembers the target it began toward")
			assert.Equal(t, target, recorded)
		})
	}
}

// A check that records a newer Available while the target is being staged
// does not change what the update runs toward: ${REF} names the target the
// bracket staged and remembered.
func TestRuntimeExecute_Update_NewerAvailableDuringStaging_KeepsTheStagedTarget(t *testing.T) {
	staged := rollingTarget()
	newer := domain.Available{Ref: "nightly-latest", Commit: "c3"}
	f := newBracketFixture(domain.ArrowStateReady, &staged)
	refresh := f.arrow.RefreshToTargetFn
	f.arrow.RefreshToTargetFn = func(ctx context.Context, ns domain.Namespace, target domain.Available) (*domain.Arrow, error) {
		f.available = &newer
		f.arrow.GetFn = func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns, Available: &newer}, nil
		}
		return refresh(ctx, ns, target)
	}
	var targetRef string
	f.runtime.BeginUpdateFn = func(_ context.Context, _ domain.Namespace, _ map[string]string, ref string) error {
		targetRef = ref
		return nil
	}
	uc := f.usecase()

	require.NoError(t, uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil))

	assert.Equal(t, staged.Ref, targetRef)
	remembered, ok := uc.targets.Take(rollingRow)
	require.True(t, ok)
	assert.Equal(t, staged, remembered)
}

func TestRuntimeExecute_Update_CurrentRow_DoesNothing(t *testing.T) {
	f := newBracketFixture(domain.ArrowStateRunning, nil)
	uc := f.usecase()

	err := uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"check available"}, f.log.all(), "a current row is neither stopped nor updated")
	_, ok := uc.targets.Take(rollingRow)
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

// Update reports whether it started anything, so the API can answer a row
// with nothing newer as an idempotent no-op instead of promising events that
// never come.
func TestRuntimeUpdate_ReportsWhetherAnUpdateStarted(t *testing.T) {
	target := rollingTarget()
	testCases := []struct {
		name      string
		state     domain.ArrowState
		available *domain.Available
		want      bool
	}{
		{name: "something newer starts the bracket", state: domain.ArrowStateReady, available: &target, want: true},
		{name: "nothing newer starts nothing", state: domain.ArrowStateReady, want: false},
		{name: "a running row with nothing newer is left running", state: domain.ArrowStateRunning, want: false},
		{name: "a row outside the bracket runs the method", state: domain.ArrowStateAbsent, want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBracketFixture(tc.state, tc.available)
			f.runtime.BeginExecutionFn = func(context.Context, domain.Namespace, string, map[string]string) error {
				return nil
			}

			started, err := f.usecase().Update(context.Background(), rollingRow, nil)

			require.NoError(t, err)
			assert.Equal(t, tc.want, started)
		})
	}
}

func TestRuntimeUpdate_Failures_StartNothing(t *testing.T) {
	testCases := []struct {
		name    string
		vars    map[string]string
		prepare func(*bracketFixture)
		wantErr error
	}{
		{
			name: "a rejected bracket keeps its state violation",
			prepare: func(f *bracketFixture) {
				f.runtime.BeginUpdateFn = func(context.Context, domain.Namespace, map[string]string, string) error {
					return apperrors.ErrStateViolation
				}
			},
			wantErr: apperrors.ErrStateViolation,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target := rollingTarget()
			f := newBracketFixture(domain.ArrowStateReady, &target)
			if tc.prepare != nil {
				tc.prepare(f)
			}

			started, err := f.usecase().Update(context.Background(), rollingRow, tc.vars)

			require.ErrorIs(t, err, tc.wantErr)
			assert.False(t, started)
		})
	}
}

// A target that gains a dependency lands the row outdated with the new
// dependency pending, and the dependency is installed before the target's
// update steps run.
func TestRuntimeExecute_Update_TargetGainsADependency_InstallsItFirst(t *testing.T) {
	target := rollingTarget()
	dep := domain.Namespace("github.com/user/tool@v1.*")
	f := newBracketFixture(domain.ArrowStateReady, &target)
	var pending *domainRuntime.DepSyncInfo
	f.graph.DiffDepsFn = func(_, _ *domain.Arrow) models.DepDiff {
		return models.DepDiff{Added: []domain.DependencyEdge{{Namespace: dep, Constraint: "v1.*"}}}
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
			// The target is judged again: one no fetch can stage stops being
			// offered once the fetch recorded it empty.
			wantLog: []string{"check available", "refresh", "check available"},
		},
		{
			name: "target cannot be staged nor judged again",
			arrange: func(f *bracketFixture) {
				checks := 0
				f.arrow.CheckAvailableFn = func(context.Context, domain.Namespace) (*domain.Available, error) {
					f.log.add("check available")
					checks++
					if checks > 1 {
						return nil, boom
					}
					return f.available, nil
				}
				f.arrow.RefreshToTargetFn = func(context.Context, domain.Namespace, domain.Available) (*domain.Arrow, error) {
					f.log.add("refresh")
					return nil, boom
				}
			},
			wantLog: []string{"check available", "refresh", "check available"},
		},
		{
			name: "dependency change cannot be recorded",
			arrange: func(f *bracketFixture) {
				f.graph.DiffDepsFn = func(_, _ *domain.Arrow) models.DepDiff {
					return models.DepDiff{Removed: []domain.DependencyEdge{{Namespace: "github.com/user/old@v1"}}}
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
				f.runtime.BeginUpdateFn = func(context.Context, domain.Namespace, map[string]string, string) error {
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
			_, kept := uc.targets.Take(rollingRow)
			assert.False(t, kept, "an update that never began leaves no target behind")
		})
	}
}

// Once the target manifest is staged, a bracket that fails before its update
// begins must put the installed release's manifest back: nothing will end
// to restore it, and the next install would run the target's recipe for the
// installed ${REF}.
func TestRuntimeExecute_Update_FailureAfterStaging_RestoresTheInstalledManifest(t *testing.T) {
	boom := errors.New("boom")

	testCases := []struct {
		name        string
		arrange     func(f *bracketFixture, cancel context.CancelFunc)
		wantErr     error
		wantRestore bool
	}{
		{
			name: "stop fails before anything is staged",
			arrange: func(f *bracketFixture, _ context.CancelFunc) {
				f.setState(domain.ArrowStateRunning)
				f.runtime.BeginStopFn = func(context.Context, domain.Namespace) error { return boom }
			},
			wantErr: boom,
		},
		{
			name: "staging itself fails",
			arrange: func(f *bracketFixture, _ context.CancelFunc) {
				f.arrow.RefreshToTargetFn = func(context.Context, domain.Namespace, domain.Available) (*domain.Arrow, error) {
					return nil, boom
				}
			},
			wantErr: boom,
		},
		{
			name: "a dependency change cannot be recorded",
			arrange: func(f *bracketFixture, _ context.CancelFunc) {
				f.graph.DiffDepsFn = func(_, _ *domain.Arrow) models.DepDiff {
					return models.DepDiff{Added: []domain.DependencyEdge{{Namespace: "github.com/user/new@v1"}}}
				}
				f.runtime.MarkOutdatedFn = func(context.Context, domain.Namespace, []domain.Namespace, []domain.Namespace) error {
					return boom
				}
			},
			wantErr:     boom,
			wantRestore: true,
		},
		{
			name: "a gained dependency fails to sync",
			arrange: func(f *bracketFixture, _ context.CancelFunc) {
				f.setState(domain.ArrowStateOutdated)
				f.runtime.GetRuntimeFn = func(context.Context, domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
					return nil, boom
				}
			},
			wantErr:     boom,
			wantRestore: true,
		},
		{
			name: "the update cannot begin",
			arrange: func(f *bracketFixture, _ context.CancelFunc) {
				f.runtime.BeginUpdateFn = func(context.Context, domain.Namespace, map[string]string, string) error {
					return boom
				}
			},
			wantErr:     boom,
			wantRestore: true,
		},
		{
			name: "the caller gives up after staging",
			arrange: func(f *bracketFixture, cancel context.CancelFunc) {
				f.runtime.BeginUpdateFn = func(ctx context.Context, _ domain.Namespace, _ map[string]string, _ string) error {
					cancel()
					return ctx.Err()
				}
			},
			wantErr:     context.Canceled,
			wantRestore: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target := rollingTarget()
			f := newBracketFixture(domain.ArrowStateReady, &target)
			f.arrow.GetFn = func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
				return &domain.Arrow{Namespace: ns, Resolved: domain.Resolved{Ref: "nightly-latest", Commit: "c1"}}, nil
			}
			var restoredWithLiveCtx []bool
			refresh := f.arrow.RefreshToTargetFn
			f.arrow.RefreshToTargetFn = func(ctx context.Context, ns domain.Namespace, a domain.Available) (*domain.Arrow, error) {
				if a.Commit == "c1" {
					restoredWithLiveCtx = append(restoredWithLiveCtx, ctx.Err() == nil)
				}
				return refresh(ctx, ns, a)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tc.arrange(f, cancel)

			err := f.usecase().Execute(ctx, rollingRow, domain.MethodUpdate, nil)

			require.ErrorIs(t, err, tc.wantErr)
			if tc.wantRestore {
				assert.Equal(t, []bool{true}, restoredWithLiveCtx, "the installed manifest is restored once, under a context that still runs")
			} else {
				assert.Empty(t, restoredWithLiveCtx)
			}
		})
	}
}

// The caller gave up while BeginUpdate was being accepted, but the run began:
// restoring the installed manifest now would run the old recipe for the
// target ${REF}. The run owns the outcome; its end settles the row.
func TestRuntimeExecute_Update_CallerGivesUpAfterTheRunBegan_RestoresNothing(t *testing.T) {
	target := rollingTarget()
	f := newBracketFixture(domain.ArrowStateReady, &target)
	f.arrow.GetFn = func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		return &domain.Arrow{Namespace: ns, Resolved: domain.Resolved{Ref: "nightly-latest", Commit: "c1"}}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.runtime.BeginUpdateFn = func(ctx context.Context, _ domain.Namespace, _ map[string]string, _ string) error {
		f.setState(domain.ArrowStateUpdating)
		cancel()
		return ctx.Err()
	}
	uc := f.usecase()

	err := uc.Execute(ctx, rollingRow, domain.MethodUpdate, nil)

	require.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, f.log.all(), "refresh to c1", "the installed manifest must not replace the one the run is using")
	remembered, ok := uc.targets.Take(rollingRow)
	require.True(t, ok, "the run's end commits the target it began toward")
	assert.Equal(t, target, remembered)
}

// The run began and already ended (its end took the target) before the
// caller's BeginUpdate returned: the end settled the row, nothing is undone.
func TestRuntimeExecute_Update_CallerGivesUpAfterTheRunEnded_RestoresNothing(t *testing.T) {
	target := rollingTarget()
	f := newBracketFixture(domain.ArrowStateReady, &target)
	f.arrow.GetFn = func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		return &domain.Arrow{Namespace: ns, Resolved: domain.Resolved{Ref: "nightly-latest", Commit: "c1"}}, nil
	}
	uc := f.usecase()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.runtime.BeginUpdateFn = func(ctx context.Context, ns domain.Namespace, _ map[string]string, _ string) error {
		_, _ = uc.targets.Take(ns)
		cancel()
		return ctx.Err()
	}

	err := uc.Execute(ctx, rollingRow, domain.MethodUpdate, nil)

	require.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, f.log.all(), "refresh to c1")
}

// A self update the caller abandoned before it was accepted restores the
// installed manifest like any other row's.
func TestRuntimeExecute_Update_SelfRowAbandonedBeforeAcceptance_Restores(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	selfRow := self.WithRef("stable")
	target := rollingTarget()
	f := newBracketFixture(domain.ArrowStateReady, &target)
	f.arrow.GetFn = func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		return &domain.Arrow{Namespace: ns, Resolved: domain.Resolved{Ref: "stable-26.5.1", Commit: "c1"}}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.runtime.BeginUpdateFn = func(ctx context.Context, _ domain.Namespace, _ map[string]string, _ string) error {
		cancel()
		return ctx.Err()
	}

	err := f.usecase().Execute(ctx, selfRow, domain.MethodUpdate, nil)

	require.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, f.log.all(), "refresh to c1", "the staged target manifest is put back")
}

// quiver.core's own update builds a release like any other, so a checksum
// retry during it must find the target the run began toward.
func TestRuntimeUpdate_SelfNamespace_RemembersTheTargetItsRunBuilds(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	selfRow := self.WithRef("nightly-latest")
	target := rollingTarget()
	f := newBracketFixture(domain.ArrowStateReady, &target)
	uc := f.usecase()

	require.NoError(t, uc.Execute(context.Background(), selfRow, domain.MethodUpdate, nil))

	peeked, remembered := uc.targets.Peek(selfRow)
	assert.True(t, remembered)
	assert.Equal(t, target, peeked)
}

// quiver.core's update ends like any other: its end commits the target and
// releases the one the bracket remembered.
func TestRuntimeUpdate_SelfNamespace_EndCommitsTheRememberedTarget(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	selfRow := self.WithRef("stable")
	target := rollingTarget()
	f := newBracketFixture(domain.ArrowStateReady, &target)
	a, rt, log := commitFixture(true, nil)
	f.arrow.TargetUnmovedFn = a.TargetUnmovedFn
	f.arrow.AdvanceFn = a.AdvanceFn
	f.runtime.ReconcileVersionBadgeFn = rt.ReconcileVersionBadgeFn
	uc := f.usecase()

	require.NoError(t, uc.Execute(context.Background(), selfRow, domain.MethodUpdate, nil))
	uc.onUpdateEnded(context.Background(), updateEnded(selfRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Contains(t, f.log.all(), "begin update")
	_, remembered := uc.targets.Take(selfRow)
	assert.False(t, remembered)
	assert.Equal(t, []string{"re-resolve c2", "advance c2", "reconcile badge"}, log.all())
}

// quiver.core's own update is no exception to the bracket: a self row that is
// already at its selector's target runs no update steps, so the self-update
// swap never starts for it. Only a selector that moved ahead of what is
// installed begins an update.
func TestRuntimeUpdate_SelfNamespace_CurrentRowRunsNoSteps(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	selfRow := self.WithRef("v1")
	f := newBracketFixture(domain.ArrowStateReady, nil)

	require.NoError(t, f.usecase().Execute(context.Background(), selfRow, domain.MethodUpdate, nil))

	assert.Equal(t, []string{"check available"}, f.log.all())
}

// holdFirstBracket starts an update that stays inside its bracket until the
// returned release is called, and reports a second bracket waiting on it.
func holdFirstBracket(t *testing.T, f *bracketFixture, uc *harness) (waiting <-chan struct{}, release func(), firstErr func() error) {
	t.Helper()
	entered := make(chan struct{})
	unblock := make(chan struct{})
	refresh := f.arrow.RefreshToTargetFn
	var once sync.Once
	f.arrow.RefreshToTargetFn = func(ctx context.Context, ns domain.Namespace, target domain.Available) (*domain.Arrow, error) {
		first := false
		once.Do(func() { first = true })
		if first {
			close(entered)
			<-unblock
		}
		return refresh(ctx, ns, target)
	}
	waitingCh := make(chan struct{})
	var waitOnce sync.Once
	uc.targets.onWait = func(domain.Namespace) { waitOnce.Do(func() { close(waitingCh) }) }

	done := make(chan error, 1)
	go func() { done <- uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil) }()
	<-entered
	return waitingCh, func() { close(unblock) }, func() error { return <-done }
}

// A second update of the same identity waits on the first bracket, and by
// the time it gets in, the first update is running: it is rejected without
// re-resolving, staging a manifest or beginning anything.
func TestRuntimeExecute_Update_SecondBracketWaitsForTheFirst(t *testing.T) {
	first := domain.Available{Ref: "nightly-latest", Commit: "c2"}
	f := newBracketFixture(domain.ArrowStateReady, &first)
	f.runtime.BeginUpdateFn = func(context.Context, domain.Namespace, map[string]string, string) error {
		f.log.add("begin update")
		f.setState(domain.ArrowStateUpdating)
		return nil
	}
	uc := f.usecase()
	waiting, release, firstErr := holdFirstBracket(t, f, uc)

	secondErr := make(chan error, 1)
	go func() { secondErr <- uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil) }()
	<-waiting
	release()

	require.NoError(t, firstErr())
	require.ErrorIs(t, <-secondErr, apperrors.ErrStateViolation)
	assert.Equal(t, []string{"check available", "refresh to c2", "begin update"}, f.log.all())
	remembered, ok := uc.targets.Take(rollingRow)
	require.True(t, ok)
	assert.Equal(t, first, remembered)
}

// A caller that gives up while waiting for the row's bracket is not held for
// the whole of the first update.
func TestRuntimeExecute_Update_WaitingCallerCanGiveUp(t *testing.T) {
	target := rollingTarget()
	f := newBracketFixture(domain.ArrowStateReady, &target)
	uc := f.usecase()
	waiting, release, firstErr := holdFirstBracket(t, f, uc)

	ctx, cancel := context.WithCancel(context.Background())
	secondErr := make(chan error, 1)
	go func() { secondErr <- uc.Execute(ctx, rollingRow, domain.MethodUpdate, nil) }()
	<-waiting
	cancel()

	require.ErrorIs(t, <-secondErr, context.Canceled)
	release()
	require.NoError(t, firstErr())
	assert.Equal(t, []string{"check available", "refresh to c2", "begin update"}, f.log.all())
}

// An update has ended (the row is Ready again) but its end handler has not
// run yet. A new update must not start: it would replace the remembered
// target the pending handler is about to commit. Once the handler runs, it
// commits its own target.
func TestRuntimeExecute_Update_PendingEndHandlerRejectsTheNextBracket(t *testing.T) {
	first := domain.Available{Ref: "nightly-latest", Commit: "c1"}
	moved := domain.Available{Ref: "nightly-latest", Commit: "c2"}
	f := newBracketFixture(domain.ArrowStateReady, &first)
	a, rt, commits := commitFixture(true, nil)
	f.arrow.TargetUnmovedFn = a.TargetUnmovedFn
	f.arrow.AdvanceFn = a.AdvanceFn
	f.runtime.ReconcileVersionBadgeFn = rt.ReconcileVersionBadgeFn
	uc := f.usecase()
	require.NoError(t, uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil))

	f.available = &moved
	err := uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil)

	require.ErrorIs(t, err, apperrors.ErrStateViolation)
	assert.Equal(t, []string{"check available", "refresh to c1", "begin update"}, f.log.all())

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Equal(t, []string{"re-resolve c1", "advance c1", "reconcile badge"}, commits.all())
}

// The first update's steps ended and its detached commit is still landing
// (the row reads settling). A second update must not begin: it would run the
// update steps again toward the target the first one already installed.
func TestRuntimeExecute_Update_CommitInFlightRejectsTheNextBracket(t *testing.T) {
	target := rollingTarget()
	f := newBracketFixture(domain.ArrowStateReady, &target)
	uc := f.usecase()
	require.True(t, uc.commits.Begin(rollingRow))
	defer uc.commits.Done(rollingRow)

	err := uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil)

	require.ErrorIs(t, err, apperrors.ErrStateViolation)
	assert.Empty(t, f.log.all(), "nothing is checked, staged or begun while the row settles")
	assert.True(t, uc.Settling(rollingRow))
}

func TestRuntimeExecute_Normal(t *testing.T) {
	called := false
	rt := &mocks.MockRuntime{
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, method string, _ map[string]string) error {
			called = true
			if method != "start" {
				t.Errorf("expected method 'start', got %q", method)
			}
			return nil
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.Execute(context.Background(), "test/arrow@v1", "start", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected BeginExecution to be called")
	}
}

func TestRuntimeExecute_Update_GetStateError_ReturnsError(t *testing.T) {
	stateErr := errors.New("state error")
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return "", stateErr
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.Execute(context.Background(), "test/arrow@v1", domain.MethodUpdate, nil); !errors.Is(err, stateErr) {
		t.Fatalf("expected stateErr, got %v", err)
	}
}

func TestRuntimeUsecase_Reset_ForgetsRuntime(t *testing.T) {
	ns := domain.Namespace("github.com/u/stuck@main")
	rt := &mocks.MockRuntime{}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})

	err := uc.Reset(context.Background(), ns)

	require.NoError(t, err)
	if len(rt.ForgottenNamespaces) == 0 || rt.ForgottenNamespaces[0] != ns {
		t.Fatalf("expected Reset to call Forget with %s, got %v", ns, rt.ForgottenNamespaces)
	}
}

func TestRuntimeUsecase_Reset_PropagatesForgetError(t *testing.T) {
	ns := domain.Namespace("github.com/u/stuck@main")
	rt := &mocks.MockRuntime{
		ForgetErr: assert.AnError,
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})

	err := uc.Reset(context.Background(), ns)

	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

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

func (d *depRows) runtime() *mocks.MockRuntime {
	return &mocks.MockRuntime{
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
func depArrows(adds map[domain.Namespace]domain.Namespace, staged func(domain.Namespace)) (*mocks.MockArrow, *mocks.MockGraph) {
	target := rollingTarget()
	a := &mocks.MockArrow{
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
	g := &mocks.MockGraph{
		DiffDepsFn: func(_, next *domain.Arrow) models.DepDiff {
			return models.DepDiff{Added: []domain.DependencyEdge{{Namespace: adds[next.Namespace]}}}
		},
	}
	return a, g
}

// failOnBracketWait makes any wait on a row's bracket fail the test and end
// every caller: an update installing its dependencies must never wait on a
// bracket, or two of them can wait on each other forever.
func failOnBracketWait(t *testing.T, uc *harness, cancel context.CancelFunc) {
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
			g.ResolveFn = func(context.Context, domain.Namespace) (models.Plan, error) {
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
