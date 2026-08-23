package usecases

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func newUC(a *ucmocks.MockArrow, rt *ucmocks.MockRuntime, g *ucmocks.MockGraph) *runtimeUsecase {
	return &runtimeUsecase{arrow: a, runtime: rt, graph: g}
}

// --- tests ---

func TestRuntimeUninstall_DependentsGuard(t *testing.T) {
	g := &ucmocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) {
			return true, nil
		},
	}
	rt := &ucmocks.MockRuntime{}
	uc := &runtimeUsecase{
		arrow:   &ucmocks.MockArrow{},
		runtime: rt,
		graph:   g,
	}

	err := uc.Uninstall(context.Background(), "test/arrow@v1", nil)

	if !errors.Is(err, apperrors.ErrDependentsExist) {
		t.Fatalf("expected ErrDependentsExist, got %v", err)
	}
}

func TestRuntimeUninstall_SuccessWhenNoDependents(t *testing.T) {
	beginCalled := false
	g := &ucmocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) {
			return false, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		BeginUninstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	uc := &runtimeUsecase{
		arrow:   &ucmocks.MockArrow{},
		runtime: rt,
		graph:   g,
	}

	err := uc.Uninstall(context.Background(), "test/arrow@v1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !beginCalled {
		t.Fatal("expected BeginUninstall to be called")
	}
}

func TestRuntimeOnEnded_DrainCascade(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	stopCalled := false

	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == depNs {
				return domain.ArrowStateRunning, nil
			}
			return domain.ArrowStateAbsent, nil
		},
		BeginStopFn: func(_ context.Context, ns domain.Namespace) error {
			if ns == depNs {
				stopCalled = true
			}
			return nil
		},
	}
	uc := &runtimeUsecase{
		arrow:   &ucmocks.MockArrow{},
		runtime: rt,
		graph:   g,
	}

	rt2 := domainRuntime.ArrowRuntime{
		Ref: "test/app@v1",
		LastReturn: &domainRuntime.Return{
			Method:  domain.MethodStop,
			Outcome: domainRuntime.ExecutionOutcomeSuccess,
		},
	}
	uc.onRuntimeEnded(context.Background(), rt2)

	if !stopCalled {
		t.Fatal("expected Stop to be called on dep with no other running parents")
	}
}

func TestRuntimeOnEnded_DrainCascade_ExcludesStoppedRef(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	stoppedRef := domain.Namespace("test/app@v1")
	stopCalled := false

	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return []domain.Namespace{stoppedRef}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == depNs {
				return domain.ArrowStateRunning, nil
			}
			if ns == stoppedRef {
				return domain.ArrowStateRunning, nil
			}
			return domain.ArrowStateAbsent, nil
		},
		BeginStopFn: func(_ context.Context, ns domain.Namespace) error {
			if ns == depNs {
				stopCalled = true
			}
			return nil
		},
	}
	uc := &runtimeUsecase{
		arrow:   &ucmocks.MockArrow{},
		runtime: rt,
		graph:   g,
	}

	rt2 := domainRuntime.ArrowRuntime{
		Ref: stoppedRef,
		LastReturn: &domainRuntime.Return{
			Method:  domain.MethodStop,
			Outcome: domainRuntime.ExecutionOutcomeSuccess,
		},
	}
	uc.onRuntimeEnded(context.Background(), rt2)

	if !stopCalled {
		t.Fatal("expected Stop to be called on dep even when its only parent (stoppedRef) is excluded from the count")
	}
}

// ─── NewRuntimeUsecase ────────────────────────────────────────────────────────

func TestRuntimeNewUsecase_DoesNotPanic(t *testing.T) {
	uc := NewRuntimeUsecase(&ucmocks.MockArrow{}, &ucmocks.MockRuntime{}, &ucmocks.MockGraph{})
	if uc == nil {
		t.Fatal("expected non-nil usecase")
	}
}

// ─── Stop / RuntimeExists / Start / Shutdown ──────────────────────────────────

func TestRuntimeStop_DelegatesToRuntime(t *testing.T) {
	called := false
	rt := &ucmocks.MockRuntime{
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			called = true
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.Stop(context.Background(), "test/arrow@v1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected runtime.BeginStop to be called")
	}
}

func TestRuntimeRuntimeExists_DelegatesToRuntime(t *testing.T) {
	rt := &ucmocks.MockRuntime{
		RuntimeExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) {
			return true, nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	exists, err := uc.RuntimeExists(context.Background(), "test/arrow@v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Fatal("expected true")
	}
}

func TestRuntimeStart_DelegatesToRuntime(t *testing.T) {
	called := false
	rt := &ucmocks.MockRuntime{
		StartFn: func(_ context.Context) { called = true },
	}
	newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{}).Start(context.Background())
	if !called {
		t.Fatal("expected runtime.Start to be called")
	}
}

// ─── Install ──────────────────────────────────────────────────────────────────

func TestRuntimeInstall_NotExists(t *testing.T) {
	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, nil },
	}
	uc := newUC(a, &ucmocks.MockRuntime{}, &ucmocks.MockGraph{})
	if _, err := uc.Install(context.Background(), "test/arrow@v1", nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRuntimeInstall_ExistsError(t *testing.T) {
	expected := errors.New("exists error")
	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, expected },
	}
	uc := newUC(a, &ucmocks.MockRuntime{}, &ucmocks.MockGraph{})
	if _, err := uc.Install(context.Background(), "test/arrow@v1", nil); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestRuntimeInstall_GraphResolveError(t *testing.T) {
	expected := errors.New("graph error")
	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return nil, expected
		},
	}
	uc := newUC(a, &ucmocks.MockRuntime{}, g)
	if _, err := uc.Install(context.Background(), "test/arrow@v1", nil); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestRuntimeInstall_NoDeps_Success(t *testing.T) {
	beginCalled := false
	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		BeginInstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	uc := newUC(a, rt, g)
	if _, err := uc.Install(context.Background(), "test/arrow@v1", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !beginCalled {
		t.Fatal("expected BeginInstall to be called")
	}
}

func TestRuntimeInstall_AlreadyReady_IsIdempotent(t *testing.T) {
	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		BeginInstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			t.Fatal("BeginInstall must not be called when arrow is already Ready")
			return nil
		},
	}
	uc := newUC(a, rt, g)
	if _, err := uc.Install(context.Background(), "test/arrow@v1", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRuntimeInstall_NoOpReturnsStartedFalse(t *testing.T) {
	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
	}
	uc := newUC(a, rt, g)

	started, err := uc.Install(context.Background(), "test/arrow@v1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if started {
		t.Fatal("expected started=false when arrow is already Ready")
	}
}

func TestRuntimeInstall_StartReturnsStartedTrue(t *testing.T) {
	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
	}
	uc := newUC(a, rt, g)

	started, err := uc.Install(context.Background(), "test/arrow@v1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !started {
		t.Fatal("expected started=true when a fresh install begins")
	}
}

func TestRuntimeInstall_DepAlreadyInstalled(t *testing.T) {
	mainNs := domain.Namespace("test/main@v1")
	depNs := domain.Namespace("test/dep@v1")
	beginCalled := false

	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, ns domain.Namespace) (graph.Plan, error) {
			if ns == mainNs {
				return graph.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
			}
			return graph.Plan{}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == depNs {
				return domain.ArrowStateReady, nil
			}
			return domain.ArrowStateAbsent, nil
		},
		BeginInstallFn: func(_ context.Context, ns domain.Namespace, _ map[string]string) error {
			if ns == mainNs {
				beginCalled = true
			}
			return nil
		},
	}
	uc := newUC(a, rt, g)
	if _, err := uc.Install(context.Background(), mainNs, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !beginCalled {
		t.Fatal("expected main BeginInstall to be called")
	}
}

// ─── installOneDep ────────────────────────────────────────────────────────────

func TestInstallOneDep_AlreadyReady_NoWait(t *testing.T) {
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.installOneDep(context.Background(), "test/dep@v1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallOneDep_WaitsForSuccess(t *testing.T) {
	result := domainRuntime.ArrowRuntime{
		LastReturn: &domainRuntime.Return{
			Method:  domain.MethodInstall,
			Outcome: domainRuntime.ExecutionOutcomeSuccess,
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			ch := make(chan domainRuntime.ArrowRuntime, 1)
			ch <- result
			return ch, func() {}, nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.installOneDep(context.Background(), "test/dep@v1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallOneDep_WaitsForFailure(t *testing.T) {
	result := domainRuntime.ArrowRuntime{
		LastReturn: &domainRuntime.Return{
			Method:  domain.MethodInstall,
			Outcome: domainRuntime.ExecutionOutcomeFailed,
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			ch := make(chan domainRuntime.ArrowRuntime, 1)
			ch <- result
			return ch, func() {}, nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.installOneDep(context.Background(), "test/dep@v1"); err == nil {
		t.Fatal("expected error for failed install")
	}
}

func TestInstallOneDep_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			cancel()
			return domain.ArrowStateAbsent, nil
		},
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			return make(chan domainRuntime.ArrowRuntime), func() {}, nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.installOneDep(ctx, "test/dep@v1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// ─── startServiceDep ─────────────────────────────────────────────────────────

func TestStartServiceDep_AlreadyRunning_NoOp(t *testing.T) {
	beginCalled := false
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateRunning, nil
		},
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.startServiceDep(context.Background(), "test/dep@v1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if beginCalled {
		t.Fatal("expected BeginExecution NOT to be called when already running")
	}
}

func TestStartServiceDep_NotRunning_StartsExecution(t *testing.T) {
	beginCalled := false
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, method string, _ map[string]string) error {
			if method == domain.MethodExecute {
				beginCalled = true
			}
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.startServiceDep(context.Background(), "test/dep@v1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !beginCalled {
		t.Fatal("expected BeginExecution to be called with MethodExecute")
	}
}

// ─── Execute ──────────────────────────────────────────────────────────────────

func TestRuntimeExecute_Normal(t *testing.T) {
	called := false
	rt := &ucmocks.MockRuntime{
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, method string, _ map[string]string) error {
			called = true
			if method != "start" {
				t.Errorf("expected method 'start', got %q", method)
			}
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.Execute(context.Background(), "test/arrow@v1", "start", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected BeginExecution to be called")
	}
}

func TestRuntimeExecute_Update_Ready_CallsBeginUpdate(t *testing.T) {
	updateCalled := false
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		BeginUpdateFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			updateCalled = true
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.Execute(context.Background(), "test/arrow@v1", domain.MethodUpdate, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updateCalled {
		t.Fatal("expected BeginUpdate to be called for Ready state")
	}
}

func TestRuntimeExecute_Update_Outdated_NoPendingSync(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	updateCalled := false

	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateOutdated, nil
		},
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:            ns,
				State:          domain.ArrowStateOutdated,
				PendingDepSync: nil,
			}, nil
		},
		BeginUpdateFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			updateCalled = true
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.Execute(context.Background(), ns, domain.MethodUpdate, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updateCalled {
		t.Fatal("expected BeginUpdate to be called")
	}
}

// ─── onArrowUpgraded ─────────────────────────────────────────────────────────

func TestRuntimeOnArrowUpgraded_OldArrowNotFound_NoOp(t *testing.T) {
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return nil, nil },
	}
	uc := newUC(a, &ucmocks.MockRuntime{}, &ucmocks.MockGraph{})
	uc.onArrowUpgraded(context.Background(), domain.Arrow{
		Namespace:      "test/new@v2",
		UpgradedFromNs: "test/old@v1",
	})
}

func TestRuntimeOnArrowUpgraded_OldStateNotReady_JustRemoves(t *testing.T) {
	removeCalled := false
	beginCalled := false

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		RemoveFn: func(_ context.Context, _ domain.Namespace) error {
			removeCalled = true
			return nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateInstalling, nil
		},
		BeginInstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	g := &ucmocks.MockGraph{
		DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff { return graph.DepDiff{} },
	}
	newUC(a, rt, g).onArrowUpgraded(context.Background(), domain.Arrow{
		Namespace: "test/new@v2", UpgradedFromNs: "test/old@v1",
	})
	if !removeCalled {
		t.Fatal("expected arrow.Remove to be called")
	}
	if beginCalled {
		t.Fatal("expected no BeginInstall when old state is not Ready")
	}
}

func TestRuntimeOnArrowUpgraded_ReadyNoDiff_BeginInstall(t *testing.T) {
	beginCalled := false
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		RemoveFn: func(_ context.Context, _ domain.Namespace) error { return nil },
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		BeginInstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	g := &ucmocks.MockGraph{
		DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff { return graph.DepDiff{} },
	}
	newUC(a, rt, g).onArrowUpgraded(context.Background(), domain.Arrow{
		Namespace: "test/new@v2", UpgradedFromNs: "test/old@v1",
	})
	if !beginCalled {
		t.Fatal("expected BeginInstall to be called")
	}
}

func TestRuntimeOnArrowUpgraded_ReadyWithDiff_MarkOutdated(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	markCalled := false

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		RemoveFn: func(_ context.Context, _ domain.Namespace) error { return nil },
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		MarkOutdatedFn: func(_ context.Context, _ domain.Namespace, added, _ []domain.Namespace) error {
			markCalled = true
			if len(added) == 0 || added[0] != depNs {
				t.Errorf("unexpected added deps: %v", added)
			}
			return nil
		},
	}
	g := &ucmocks.MockGraph{
		DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff {
			return graph.DepDiff{Added: []domain.DependencyEdge{{Namespace: depNs}}}
		},
	}
	newUC(a, rt, g).onArrowUpgraded(context.Background(), domain.Arrow{
		Namespace: "test/new@v2", UpgradedFromNs: "test/old@v1",
	})
	if !markCalled {
		t.Fatal("expected MarkOutdated to be called")
	}
}

// ─── onRuntimeEnded ───────────────────────────────────────────────────────────

func TestRuntimeOnEnded_NilLastReturn_NoOp(t *testing.T) {
	stopCalled := false
	rt := &ucmocks.MockRuntime{
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			stopCalled = true
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{Ref: "test/arrow@v1"})
	if stopCalled {
		t.Fatal("expected no side effects for nil LastReturn")
	}
}

func TestRuntimeOnEnded_MethodInstall_NoOp(t *testing.T) {
	stopCalled := false
	rt := &ucmocks.MockRuntime{
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			stopCalled = true
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/arrow@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodInstall},
	})
	if stopCalled {
		t.Fatal("expected no Stop call for MethodInstall")
	}
}

// ─── onUninstallEnded ─────────────────────────────────────────────────────────

func TestRuntimeOnUninstallEnded_DepAbsent_NoAction(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	stopCalled := false

	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			stopCalled = true
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodUninstall},
	})
	if stopCalled {
		t.Fatal("expected no Stop for absent dep")
	}
}

func TestRuntimeOnUninstallEnded_DepRunning_Stops(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	stopCalled := false

	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == depNs {
				return domain.ArrowStateRunning, nil
			}
			return domain.ArrowStateAbsent, nil
		},
		BeginStopFn: func(_ context.Context, ns domain.Namespace) error {
			if ns == depNs {
				stopCalled = true
			}
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{UserInstalled: false}, nil
		},
	}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodUninstall},
	})
	if !stopCalled {
		t.Fatal("expected Stop to be called for running dep")
	}
}

func TestRuntimeOnUninstallEnded_DepReady_Uninstalls(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	beginCalled := false

	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == depNs {
				return domain.ArrowStateReady, nil
			}
			return domain.ArrowStateAbsent, nil
		},
		BeginUninstallFn: func(_ context.Context, ns domain.Namespace, _ map[string]string) error {
			if ns == depNs {
				beginCalled = true
			}
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{UserInstalled: false}, nil
		},
	}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodUninstall},
	})
	if !beginCalled {
		t.Fatal("expected BeginUninstall for ready dep")
	}
}

func TestRuntimeOnUninstallEnded_DepHasOtherRunningParent_NoAction(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	otherParent := domain.Namespace("test/other@v1")
	stopCalled := false

	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return []domain.Namespace{otherParent}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			switch ns {
			case depNs:
				return domain.ArrowStateRunning, nil
			case otherParent:
				return domain.ArrowStateRunning, nil // still running
			}
			return domain.ArrowStateAbsent, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			stopCalled = true
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodUninstall},
	})
	if stopCalled {
		t.Fatal("expected no Stop when dep still has other running parents")
	}
}

// ─── maybeAutoUninstallStopped ────────────────────────────────────────────────

func TestMaybeAutoUninstallStopped_UserInstalled_NoOp(t *testing.T) {
	beginCalled := false
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{UserInstalled: true}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		BeginUninstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	newUC(a, rt, &ucmocks.MockGraph{}).maybeAutoUninstallStopped(context.Background(), "test/arrow@v1")
	if beginCalled {
		t.Fatal("expected no uninstall for user-installed arrow")
	}
}

func TestMaybeAutoUninstallStopped_GetArrowNil_NoOp(t *testing.T) {
	beginCalled := false
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return nil, nil },
	}
	rt := &ucmocks.MockRuntime{
		BeginUninstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	newUC(a, rt, &ucmocks.MockGraph{}).maybeAutoUninstallStopped(context.Background(), "test/arrow@v1")
	if beginCalled {
		t.Fatal("expected no uninstall when arrow not found")
	}
}

func TestMaybeAutoUninstallStopped_ParentsStillRunning_NoOp(t *testing.T) {
	parentNs := domain.Namespace("test/parent@v1")
	beginCalled := false

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{UserInstalled: false}, nil
		},
	}
	g := &ucmocks.MockGraph{
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return []domain.Namespace{parentNs}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == parentNs {
				return domain.ArrowStateRunning, nil
			}
			return domain.ArrowStateAbsent, nil
		},
		BeginUninstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	newUC(a, rt, g).maybeAutoUninstallStopped(context.Background(), "test/arrow@v1")
	if beginCalled {
		t.Fatal("expected no uninstall when parents still running")
	}
}

func TestMaybeAutoUninstallStopped_NoRunningParents_Uninstalls(t *testing.T) {
	beginCalled := false
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{UserInstalled: false}, nil
		},
	}
	g := &ucmocks.MockGraph{
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		BeginUninstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	newUC(a, rt, g).maybeAutoUninstallStopped(context.Background(), "test/arrow@v1")
	if !beginCalled {
		t.Fatal("expected BeginUninstall to be called")
	}
}

// ─── Install: missing paths ───────────────────────────────────────────────────

func TestRuntimeInstall_DepExistsError_ReturnsError(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	mainNs := domain.Namespace("test/main@v1")
	depErr := errors.New("dep check error")

	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, ns domain.Namespace) (bool, error) {
			if ns == mainNs {
				return true, nil
			}
			return false, depErr
		},
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
	}
	uc := newUC(a, &ucmocks.MockRuntime{}, g)
	if _, err := uc.Install(context.Background(), mainNs, nil); !errors.Is(err, depErr) {
		t.Fatalf("expected dep check error, got %v", err)
	}
}

func TestRuntimeInstall_ResolveForInstallError_ReturnsError(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	mainNs := domain.Namespace("test/main@v1")
	resolveErr := errors.New("resolve error")

	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, ns domain.Namespace) (bool, error) {
			if ns == mainNs {
				return true, nil
			}
			return false, nil // dep doesn't exist
		},
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return "", nil, "", resolveErr
		},
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
	}
	uc := newUC(a, &ucmocks.MockRuntime{}, g)
	if _, err := uc.Install(context.Background(), mainNs, nil); !errors.Is(err, resolveErr) {
		t.Fatalf("expected resolve error, got %v", err)
	}
}

func TestRuntimeInstall_AddDepError_ReturnsError(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	mainNs := domain.Namespace("test/main@v1")
	addErr := errors.New("add dep error")

	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, ns domain.Namespace) (bool, error) {
			if ns == mainNs {
				return true, nil
			}
			return false, nil
		},
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return depNs, &domain.Arrow{Namespace: depNs}, "", nil
		},
		AddDepFn: func(_ context.Context, _ domain.Namespace, _ *domain.Arrow, _ string) error {
			return addErr
		},
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
	}
	uc := newUC(a, &ucmocks.MockRuntime{}, g)
	if _, err := uc.Install(context.Background(), mainNs, nil); !errors.Is(err, addErr) {
		t.Fatalf("expected add dep error, got %v", err)
	}
}

func TestRuntimeInstall_AddDepAlreadyExists_Continues(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	mainNs := domain.Namespace("test/main@v1")
	beginCalled := false

	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, ns domain.Namespace) (bool, error) {
			if ns == mainNs {
				return true, nil
			}
			return false, nil
		},
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return depNs, &domain.Arrow{Namespace: depNs}, "", nil
		},
		AddDepFn: func(_ context.Context, _ domain.Namespace, _ *domain.Arrow, _ string) error {
			return apperrors.ErrAlreadyExists
		},
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == depNs {
				return domain.ArrowStateReady, nil
			}
			return domain.ArrowStateAbsent, nil
		},
		BeginInstallFn: func(_ context.Context, ns domain.Namespace, _ map[string]string) error {
			if ns == mainNs {
				beginCalled = true
			}
			return nil
		},
	}
	uc := newUC(a, rt, g)
	if _, err := uc.Install(context.Background(), mainNs, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !beginCalled {
		t.Fatal("expected main BeginInstall after ErrAlreadyExists on AddDep")
	}
}

func TestRuntimeInstall_InstallOneDepError_ReturnsError(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	mainNs := domain.Namespace("test/main@v1")
	listenErr := errors.New("listen error")

	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			return nil, nil, listenErr
		},
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
	}
	uc := newUC(a, rt, g)
	if _, err := uc.Install(context.Background(), mainNs, nil); err == nil {
		t.Fatal("expected error from installOneDep")
	}
}

func TestRuntimeInstall_ServiceDep_StartError_ReturnsError(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	mainNs := domain.Namespace("test/main@v1")
	startErr := errors.New("start error")

	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == depNs {
				return domain.ArrowStateReady, nil // already installed, skip installOneDep
			}
			return domain.ArrowStateAbsent, nil
		},
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, method string, _ map[string]string) error {
			if method == domain.MethodExecute {
				return startErr
			}
			return nil
		},
	}
	uc := newUC(a, rt, g)
	if _, err := uc.Install(context.Background(), mainNs, nil); !errors.Is(err, startErr) {
		t.Fatalf("expected startErr, got %v", err)
	}
}

// ─── installOneDep: ListenEnded error ─────────────────────────────────────────

func TestInstallOneDep_ListenEndedError_ReturnsError(t *testing.T) {
	listenErr := errors.New("listen error")
	rt := &ucmocks.MockRuntime{
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			return nil, nil, listenErr
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.installOneDep(context.Background(), "test/dep@v1"); !errors.Is(err, listenErr) {
		t.Fatalf("expected listen error, got %v", err)
	}
}

// ─── startServiceDep: missing paths ──────────────────────────────────────────

func TestStartServiceDep_GetStateError_ReturnsError(t *testing.T) {
	stateErr := errors.New("state error")
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return "", stateErr
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.startServiceDep(context.Background(), "test/dep@v1"); !errors.Is(err, stateErr) {
		t.Fatalf("expected state error, got %v", err)
	}
}

func TestStartServiceDep_BeginExecutionNonStateViolationError(t *testing.T) {
	execErr := errors.New("exec error")
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
			return execErr
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.startServiceDep(context.Background(), "test/dep@v1"); !errors.Is(err, execErr) {
		t.Fatalf("expected execErr, got %v", err)
	}
}

func TestStartServiceDep_BeginExecutionStateViolation_NoError(t *testing.T) {
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
			return apperrors.ErrStateViolation
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.startServiceDep(context.Background(), "test/dep@v1"); err != nil {
		t.Fatalf("expected no error for state violation, got %v", err)
	}
}

// ─── Execute: missing paths ───────────────────────────────────────────────────

func TestRuntimeExecute_Update_Outdated_SyncDepsError(t *testing.T) {
	syncErr := errors.New("sync error")
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateOutdated, nil
		},
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return nil, syncErr
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.Execute(context.Background(), "test/arrow@v1", domain.MethodUpdate, nil); err == nil {
		t.Fatal("expected error from syncDeps")
	}
}

func TestRuntimeExecute_Update_Outdated_BeginError(t *testing.T) {
	execErr := errors.New("begin error")
	ns := domain.Namespace("test/arrow@v1")

	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateOutdated, nil
		},
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:            ns,
				State:          domain.ArrowStateOutdated,
				PendingDepSync: nil,
			}, nil
		},
		BeginUpdateFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			return execErr
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.Execute(context.Background(), ns, domain.MethodUpdate, nil); !errors.Is(err, execErr) {
		t.Fatalf("expected execErr, got %v", err)
	}
}

// ─── syncDeps: missing paths ──────────────────────────────────────────────────

func TestRuntimeSyncDeps_GetRuntimeError_ReturnsError(t *testing.T) {
	getRtErr := errors.New("get runtime error")
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return nil, getRtErr
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.syncDeps(context.Background(), "test/arrow@v1"); !errors.Is(err, getRtErr) {
		t.Fatalf("expected getRtErr, got %v", err)
	}
}

// ─── onStopEnded: GetDependents error path ───────────────────────────────────

func TestRuntimeOnStopEnded_GetDependentsError_Continues(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	stopCalled := false

	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, errors.New("db error")
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == depNs {
				return domain.ArrowStateRunning, nil
			}
			return domain.ArrowStateAbsent, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			stopCalled = true
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return nil, nil },
	}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodStop},
	})
	if stopCalled {
		t.Fatal("expected no Stop when GetDependents fails")
	}
}

// ─── onUninstallEnded: GetDependents error path ───────────────────────────────

func TestRuntimeOnUninstallEnded_GetDependentsError_Continues(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	stopCalled := false

	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, errors.New("db error")
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == depNs {
				return domain.ArrowStateRunning, nil
			}
			return domain.ArrowStateAbsent, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			stopCalled = true
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodUninstall},
	})
	if stopCalled {
		t.Fatal("expected no Stop when GetDependents fails")
	}
}

// ─── countRunning ─────────────────────────────────────────────────────────────

func TestCountRunning_MixedStates(t *testing.T) {
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			switch ns {
			case "ns/running@v1":
				return domain.ArrowStateRunning, nil
			case "ns/absent@v1":
				return domain.ArrowStateAbsent, nil
			case "ns/ready@v1":
				return domain.ArrowStateReady, nil
			case "ns/empty@v1":
				return "", nil
			}
			return domain.ArrowStateAbsent, nil
		},
	}
	nss := []domain.Namespace{"ns/running@v1", "ns/absent@v1", "ns/ready@v1", "ns/empty@v1"}
	n := countRunning(context.Background(), nss, rt.GetState)
	// running + ready count; absent + "" do not
	if n != 2 {
		t.Fatalf("expected 2 running, got %d", n)
	}
}

func TestCountRunning_Empty_Zero(t *testing.T) {
	n := countRunning(context.Background(), nil, (&ucmocks.MockRuntime{}).GetState)
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

// ─── syncDeps ─────────────────────────────────────────────────────────────────

func TestRuntimeSyncDeps_RuntimeNil_StateViolation(t *testing.T) {
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return nil, nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.syncDeps(context.Background(), "test/arrow@v1"); !errors.Is(err, apperrors.ErrStateViolation) {
		t.Fatalf("expected ErrStateViolation, got %v", err)
	}
}

func TestRuntimeSyncDeps_NoPendingSync_Clears(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")

	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:            ns,
				State:          domain.ArrowStateOutdated,
				PendingDepSync: nil,
			}, nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.syncDeps(context.Background(), ns); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRuntimeSyncDeps_WithAddedDep_AlreadyExists(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")

	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:   ns,
				State: domain.ArrowStateOutdated,
				PendingDepSync: &domainRuntime.DepSyncInfo{
					AddedDeps: []domain.Namespace{depNs},
				},
			}, nil
		},
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == depNs {
				return domain.ArrowStateReady, nil
			}
			return domain.ArrowStateAbsent, nil
		},
	}
	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
	}
	uc := newUC(a, rt, g)
	if err := uc.syncDeps(context.Background(), ns); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRuntimeSyncDeps_WithRemovedDep_NotUserInstalled_Uninstalls(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	beginCalled := false

	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:   ns,
				State: domain.ArrowStateOutdated,
				PendingDepSync: &domainRuntime.DepSyncInfo{
					RemovedDeps: []domain.Namespace{depNs},
				},
			}, nil
		},
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == depNs {
				return domain.ArrowStateReady, nil
			}
			return domain.ArrowStateAbsent, nil
		},
		BeginUninstallFn: func(_ context.Context, ns domain.Namespace, _ map[string]string) error {
			if ns == depNs {
				beginCalled = true
			}
			return nil
		},
	}
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{UserInstalled: false}, nil
		},
	}
	g := &ucmocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) {
			return false, nil
		},
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{}, nil
		},
	}
	uc := newUC(a, rt, g)
	if err := uc.syncDeps(context.Background(), ns); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !beginCalled {
		t.Fatal("expected BeginUninstall to be called for removed non-user dep")
	}
}

// ─── installOneDep: additional error paths ───────────────────────────────────

func TestInstallOneDep_GetStateError_ReturnsError(t *testing.T) {
	stateErr := errors.New("state error")
	rt := &ucmocks.MockRuntime{
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			return make(chan domainRuntime.ArrowRuntime, 1), func() {}, nil
		},
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return "", stateErr
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.installOneDep(context.Background(), "test/dep@v1"); !errors.Is(err, stateErr) {
		t.Fatalf("expected stateErr, got %v", err)
	}
}

func TestInstallOneDep_BeginExecutionNonStateViolation_ReturnsError(t *testing.T) {
	beErr := errors.New("begin execution error")
	rt := &ucmocks.MockRuntime{
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			return make(chan domainRuntime.ArrowRuntime, 1), func() {}, nil
		},
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
		BeginInstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			return beErr
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.installOneDep(context.Background(), "test/dep@v1"); !errors.Is(err, beErr) {
		t.Fatalf("expected beErr, got %v", err)
	}
}

// ─── Uninstall: HasDependents error path ─────────────────────────────────────

func TestRuntimeUninstall_HasDependentsError_ReturnsError(t *testing.T) {
	depsErr := errors.New("deps error")
	g := &ucmocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) {
			return false, depsErr
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, &ucmocks.MockRuntime{}, g)
	if err := uc.Uninstall(context.Background(), "test/arrow@v1", nil); !errors.Is(err, depsErr) {
		t.Fatalf("expected depsErr, got %v", err)
	}
}

// ─── Execute: GetState error path ────────────────────────────────────────────

func TestRuntimeExecute_Update_GetStateError_ReturnsError(t *testing.T) {
	stateErr := errors.New("state error")
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return "", stateErr
		},
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})
	if err := uc.Execute(context.Background(), "test/arrow@v1", domain.MethodUpdate, nil); !errors.Is(err, stateErr) {
		t.Fatalf("expected stateErr, got %v", err)
	}
}

// ─── syncDeps: AddedDeps error paths ─────────────────────────────────────────

func TestRuntimeSyncDeps_AddedDep_ExistsError_ReturnsError(t *testing.T) {
	existsErr := errors.New("exists error")
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:   ns,
				State: domain.ArrowStateOutdated,
				PendingDepSync: &domainRuntime.DepSyncInfo{
					AddedDeps: []domain.Namespace{depNs},
				},
			}, nil
		},
	}
	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, existsErr },
	}
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	if err := uc.syncDeps(context.Background(), ns); !errors.Is(err, existsErr) {
		t.Fatalf("expected existsErr, got %v", err)
	}
}

func TestRuntimeSyncDeps_AddedDep_NotExists_ResolveError_ReturnsError(t *testing.T) {
	resolveErr := errors.New("resolve error")
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:   ns,
				State: domain.ArrowStateOutdated,
				PendingDepSync: &domainRuntime.DepSyncInfo{
					AddedDeps: []domain.Namespace{depNs},
				},
			}, nil
		},
	}
	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, nil },
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return "", nil, "", resolveErr
		},
	}
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	if err := uc.syncDeps(context.Background(), ns); !errors.Is(err, resolveErr) {
		t.Fatalf("expected resolveErr, got %v", err)
	}
}

func TestRuntimeSyncDeps_AddedDep_AddDepError_ReturnsError(t *testing.T) {
	addErr := errors.New("add dep error")
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:   ns,
				State: domain.ArrowStateOutdated,
				PendingDepSync: &domainRuntime.DepSyncInfo{
					AddedDeps: []domain.Namespace{depNs},
				},
			}, nil
		},
	}
	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, nil },
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return depNs, &domain.Arrow{Namespace: depNs}, "v1", nil
		},
		AddDepFn: func(_ context.Context, _ domain.Namespace, _ *domain.Arrow, _ string) error { return addErr },
	}
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	if err := uc.syncDeps(context.Background(), ns); !errors.Is(err, addErr) {
		t.Fatalf("expected addErr, got %v", err)
	}
}

func TestRuntimeSyncDeps_AddedDep_InstallOneDepError_ReturnsError(t *testing.T) {
	listenErr := errors.New("listen error")
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:   ns,
				State: domain.ArrowStateOutdated,
				PendingDepSync: &domainRuntime.DepSyncInfo{
					AddedDeps: []domain.Namespace{depNs},
				},
			}, nil
		},
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			return nil, nil, listenErr
		},
	}
	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	if err := uc.syncDeps(context.Background(), ns); !errors.Is(err, listenErr) {
		t.Fatalf("expected listenErr, got %v", err)
	}
}

func TestRuntimeSyncDeps_AddedServiceDep_StartError_ReturnsError(t *testing.T) {
	startErr := errors.New("start error")
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:   ns,
				State: domain.ArrowStateOutdated,
				PendingDepSync: &domainRuntime.DepSyncInfo{
					AddedDeps: []domain.Namespace{depNs},
				},
			}, nil
		},
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			return make(chan domainRuntime.ArrowRuntime, 1), func() {}, nil
		},
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, method string, _ map[string]string) error {
			if method == domain.MethodExecute {
				return startErr
			}
			return nil
		},
	}
	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
	}
	uc := newUC(a, rt, g)
	if err := uc.syncDeps(context.Background(), ns); !errors.Is(err, startErr) {
		t.Fatalf("expected startErr, got %v", err)
	}
}

// ─── syncDeps: RemovedDeps error paths ───────────────────────────────────────

func TestRuntimeSyncDeps_RemovedDep_GetArrowError_Skips(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	beginCalled := false
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:   ns,
				State: domain.ArrowStateOutdated,
				PendingDepSync: &domainRuntime.DepSyncInfo{
					RemovedDeps: []domain.Namespace{depNs},
				},
			}, nil
		},
		BeginUninstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return nil, errors.New("get error")
		},
	}
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	if err := uc.syncDeps(context.Background(), ns); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if beginCalled {
		t.Fatal("expected no BeginUninstall when Get fails")
	}
}

func TestRuntimeSyncDeps_RemovedDep_HasDependents_Skips(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	beginCalled := false
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:   ns,
				State: domain.ArrowStateOutdated,
				PendingDepSync: &domainRuntime.DepSyncInfo{
					RemovedDeps: []domain.Namespace{depNs},
				},
			}, nil
		},
		BeginUninstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: depNs, UserInstalled: false}, nil
		},
	}
	g := &ucmocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) {
			return true, nil
		},
	}
	uc := newUC(a, rt, g)
	if err := uc.syncDeps(context.Background(), ns); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if beginCalled {
		t.Fatal("expected no BeginUninstall when dep has dependents")
	}
}

func TestRuntimeSyncDeps_RemovedDep_GetStateError_Skips(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	beginCalled := false
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:   ns,
				State: domain.ArrowStateOutdated,
				PendingDepSync: &domainRuntime.DepSyncInfo{
					RemovedDeps: []domain.Namespace{depNs},
				},
			}, nil
		},
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return "", errors.New("state error")
		},
		BeginUninstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: depNs, UserInstalled: false}, nil
		},
	}
	g := &ucmocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) {
			return false, nil
		},
	}
	uc := newUC(a, rt, g)
	if err := uc.syncDeps(context.Background(), ns); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if beginCalled {
		t.Fatal("expected no BeginUninstall when GetState fails")
	}
}

func TestRuntimeSyncDeps_RemovedDep_Running_Stops(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	stopCalled := false
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:   ns,
				State: domain.ArrowStateOutdated,
				PendingDepSync: &domainRuntime.DepSyncInfo{
					RemovedDeps: []domain.Namespace{depNs},
				},
			}, nil
		},
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateRunning, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			stopCalled = true
			return nil
		},
	}
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: depNs, UserInstalled: false}, nil
		},
	}
	g := &ucmocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) {
			return false, nil
		},
	}
	uc := newUC(a, rt, g)
	if err := uc.syncDeps(context.Background(), ns); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stopCalled {
		t.Fatal("expected Stop called for running removed dep")
	}
}

// ─── onStopEnded: additional paths ───────────────────────────────────────────

func TestRuntimeOnStopEnded_GraphResolveError_NoOp(t *testing.T) {
	stopCalled := false
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return nil, errors.New("resolve error")
		},
	}
	rt := &ucmocks.MockRuntime{
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error { stopCalled = true; return nil },
	}
	uc := newUC(&ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return nil, nil },
	}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodStop},
	})
	if stopCalled {
		t.Fatal("expected no Stop when Resolve fails")
	}
}

func TestRuntimeOnStopEnded_ToolDep_Skipped(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	stopCalled := false
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) { return nil, nil },
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateRunning, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error { stopCalled = true; return nil },
	}
	uc := newUC(&ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return nil, nil },
	}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodStop},
	})
	if stopCalled {
		t.Fatal("expected no Stop for ToolDep entry")
	}
}

func TestRuntimeOnStopEnded_ServiceDep_GetStateError_Skips(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	stopCalled := false
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) { return nil, nil },
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return "", errors.New("state error")
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error { stopCalled = true; return nil },
	}
	uc := newUC(&ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return nil, nil },
	}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodStop},
	})
	if stopCalled {
		t.Fatal("expected no Stop when GetState fails")
	}
}

func TestRuntimeOnStopEnded_ServiceDep_StateNotRunning_Skips(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	stopCalled := false
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) { return nil, nil },
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error { stopCalled = true; return nil },
	}
	uc := newUC(&ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return nil, nil },
	}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodStop},
	})
	if stopCalled {
		t.Fatal("expected no Stop when dep state is not Running/Stopping")
	}
}

func TestRuntimeOnStopEnded_FilteredParentsPreservesOther(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	otherParentNs := domain.Namespace("test/other@v1")
	stopCalled := false
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, ns domain.Namespace) ([]domain.Namespace, error) {
			if ns == depNs {
				return []domain.Namespace{"test/app@v1", otherParentNs}, nil
			}
			return nil, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == depNs || ns == otherParentNs {
				return domain.ArrowStateRunning, nil
			}
			return domain.ArrowStateAbsent, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error { stopCalled = true; return nil },
	}
	uc := newUC(&ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return nil, nil },
	}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodStop},
	})
	if stopCalled {
		t.Fatal("expected no Stop when another parent is still running")
	}
}

// ─── maybeAutoUninstallStopped: GetDependents error ──────────────────────────

func TestMaybeAutoUninstallStopped_GetDependentsError_NoOp(t *testing.T) {
	ns := domain.Namespace("test/dep@v1")
	beginCalled := false
	g := &ucmocks.MockGraph{
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, errors.New("get dependents error")
		},
	}
	rt := &ucmocks.MockRuntime{
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	uc := newUC(&ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns, UserInstalled: false}, nil
		},
	}, rt, g)
	uc.maybeAutoUninstallStopped(context.Background(), ns)
	if beginCalled {
		t.Fatal("expected no BeginExecution when GetDependents fails")
	}
}

// ─── onUninstallEnded: additional paths ──────────────────────────────────────

func TestRuntimeOnUninstallEnded_GraphResolveError_NoOp(t *testing.T) {
	stopCalled := false
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return nil, errors.New("resolve error")
		},
	}
	rt := &ucmocks.MockRuntime{
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error { stopCalled = true; return nil },
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodUninstall},
	})
	if stopCalled {
		t.Fatal("expected no Stop when Resolve fails")
	}
}

func TestRuntimeOnUninstallEnded_GetStateError_Skips(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	stopCalled := false
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return graph.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) { return nil, nil },
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return "", errors.New("state error")
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error { stopCalled = true; return nil },
	}
	uc := newUC(&ucmocks.MockArrow{}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodUninstall},
	})
	if stopCalled {
		t.Fatal("expected no Stop when GetState fails")
	}
}

// ─── reserved variables ──────────────────────────────────────────────────────

// Every built-in is a fact about the execution, so a request that sets one is
// asking for something Quiver cannot honour and must be told so.
func TestRuntimeUsecase_ReservedVariable_RejectedOnEveryEntryPoint(t *testing.T) {
	entryPoints := map[string]func(uc *runtimeUsecase, vars map[string]string) error{
		"install": func(uc *runtimeUsecase, vars map[string]string) error {
			_, err := uc.Install(context.Background(), "github.com/user/repo@v1", vars)
			return err
		},
		"uninstall": func(uc *runtimeUsecase, vars map[string]string) error {
			return uc.Uninstall(context.Background(), "github.com/user/repo@v1", vars)
		},
		"execute": func(uc *runtimeUsecase, vars map[string]string) error {
			return uc.Execute(context.Background(), "github.com/user/repo@v1", domain.MethodExecute, vars)
		},
	}

	for entry, call := range entryPoints {
		for _, name := range domain.ReservedVariableNames() {
			t.Run(entry+"/"+name, func(t *testing.T) {
				reached := false
				rt := &ucmocks.MockRuntime{
					BeginInstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
						reached = true
						return nil
					},
					BeginUninstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
						reached = true
						return nil
					},
					BeginExecutionFn: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
						reached = true
						return nil
					},
				}
				uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})

				err := call(uc, map[string]string{name: "hijacked"})

				if !errors.Is(err, apperrors.ErrReservedVariable) {
					t.Fatalf("expected ErrReservedVariable, got %v", err)
				}
				if !strings.Contains(err.Error(), name) {
					t.Fatalf("error %q does not name the offending variable %q", err, name)
				}
				if reached {
					t.Fatal("request reached the runtime repository instead of being rejected")
				}
			})
		}
	}
}

// The rejection must be deterministic: a request setting several built-ins
// always names the same one, so the client sees a stable error.
func TestRuntimeUsecase_SeveralReservedVariables_NamesTheFirstInOrder(t *testing.T) {
	uc := newUC(&ucmocks.MockArrow{}, &ucmocks.MockRuntime{}, &ucmocks.MockGraph{})

	vars := map[string]string{}
	for _, name := range domain.ReservedVariableNames() {
		vars[name] = "hijacked"
	}

	for range 20 {
		_, err := uc.Install(context.Background(), "github.com/user/repo@v1", vars)
		if !strings.Contains(err.Error(), domain.ReservedVariableNames()[0]) {
			t.Fatalf("expected %q to be named, got %v", domain.ReservedVariableNames()[0], err)
		}
	}
}

func TestRuntimeUsecase_NonReservedVariable_ReachesTheRepository(t *testing.T) {
	var gotInstall, gotUninstall, gotExecute map[string]string
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
		BeginInstallFn: func(_ context.Context, _ domain.Namespace, vars map[string]string) error {
			gotInstall = vars
			return nil
		},
		BeginUninstallFn: func(_ context.Context, _ domain.Namespace, vars map[string]string) error {
			gotUninstall = vars
			return nil
		},
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, _ string, vars map[string]string) error {
			gotExecute = vars
			return nil
		},
	}
	a := &ucmocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return nil, nil
		},
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) { return false, nil },
	}
	uc := newUC(a, rt, g)

	vars := map[string]string{"PORT": "8080"}
	ns := domain.Namespace("github.com/user/repo@v1")

	if _, err := uc.Install(context.Background(), ns, vars); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := uc.Uninstall(context.Background(), ns, vars); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if err := uc.Execute(context.Background(), ns, domain.MethodExecute, vars); err != nil {
		t.Fatalf("execute: %v", err)
	}

	for name, got := range map[string]map[string]string{
		"install":   gotInstall,
		"uninstall": gotUninstall,
		"execute":   gotExecute,
	} {
		if got["PORT"] != "8080" {
			t.Fatalf("%s: expected PORT=8080 to reach the repository, got %v", name, got)
		}
	}
}

func TestRuntimeUsecase_NoVariables_IsNotRejected(t *testing.T) {
	uc := newUC(&ucmocks.MockArrow{}, &ucmocks.MockRuntime{}, &ucmocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) { return false, nil },
	})

	if err := uc.Uninstall(context.Background(), "github.com/user/repo@v1", nil); err != nil {
		t.Fatalf("expected nil vars to pass, got %v", err)
	}
}

// ─── Reset ────────────────────────────────────────────────────────────────────

func TestRuntimeUsecase_Reset_ForgetsRuntime(t *testing.T) {
	ns := domain.Namespace("github.com/u/stuck@main")
	rt := &ucmocks.MockRuntime{}
	uc := NewRuntimeUsecase(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})

	err := uc.Reset(context.Background(), ns)

	require.NoError(t, err)
	if len(rt.ForgottenNamespaces) == 0 || rt.ForgottenNamespaces[0] != ns {
		t.Fatalf("expected Reset to call Forget with %s, got %v", ns, rt.ForgottenNamespaces)
	}
}

func TestRuntimeUsecase_Reset_PropagatesForgetError(t *testing.T) {
	ns := domain.Namespace("github.com/u/stuck@main")
	rt := &ucmocks.MockRuntime{
		ForgetErr: assert.AnError,
	}
	uc := NewRuntimeUsecase(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})

	err := uc.Reset(context.Background(), ns)

	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

// ─── ListRuntimes ────────────────────────────────────────────────────────────

// A catalog view is keyed by the bare arrow identity; the installed refs live
// in Versions, and a runtime aggregate exists per ref. Looking the runtime up
// by the bare namespace never matches, so every arrow reports the synthesized
// "absent" and `quiver ps` can never show anything running.
func TestRuntimeListRuntimes_ReadsRuntimePerVersionNotBareNamespace(t *testing.T) {
	bare := domain.Namespace("github.com/user/app")
	versioned := domain.Namespace("github.com/user/app@v1")

	a := &ucmocks.MockArrow{
		ListFn: func(_ context.Context, _ *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{{
				Namespace: bare,
				Versions:  []models.VersionView{{Namespace: versioned}},
			}}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			if ns != versioned {
				return nil, nil
			}
			return &domainRuntime.ArrowRuntime{Ref: versioned, State: domain.ArrowStateRunning}, nil
		},
	}
	uc := newUC(a, rt, &ucmocks.MockGraph{})

	got, err := uc.ListRuntimes(context.Background())

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, versioned, got[0].Ref, "the runtime is keyed by the versioned namespace")
	assert.Equal(t, domain.ArrowStateRunning, got[0].State,
		"a running arrow must not be reported as absent")
}

// An arrow in the catalog that was never installed has no runtime aggregate.
// Reporting it as absent is right; dropping it from the listing is not.
func TestRuntimeListRuntimes_SynthesizesAbsentForUninstalledVersion(t *testing.T) {
	versioned := domain.Namespace("github.com/user/app@v1")

	a := &ucmocks.MockArrow{
		ListFn: func(_ context.Context, _ *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{{
				Namespace: "github.com/user/app",
				Versions:  []models.VersionView{{Namespace: versioned}},
			}}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return nil, nil
		},
	}
	uc := newUC(a, rt, &ucmocks.MockGraph{})

	got, err := uc.ListRuntimes(context.Background())

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, versioned, got[0].Ref)
	assert.Equal(t, domain.ArrowStateAbsent, got[0].State)
}

// Every installed ref is its own runtime, so an arrow with two of them
// contributes two rows rather than one.
func TestRuntimeListRuntimes_ReportsEveryVersion(t *testing.T) {
	a := &ucmocks.MockArrow{
		ListFn: func(_ context.Context, _ *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{{
				Namespace: "github.com/user/app",
				Versions: []models.VersionView{
					{Namespace: "github.com/user/app@v1"},
					{Namespace: "github.com/user/app@v2"},
				},
			}}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady}, nil
		},
	}
	uc := newUC(a, rt, &ucmocks.MockGraph{})

	got, err := uc.ListRuntimes(context.Background())

	require.NoError(t, err)
	require.Len(t, got, 2)
}

// ps is what a user runs when something is already wrong, so one unreadable
// aggregate must not take the whole listing down with it.
func TestRuntimeListRuntimes_OneUnreadableAggregateDoesNotFailTheList(t *testing.T) {
	broken := domain.Namespace("github.com/user/broken@v1")
	fine := domain.Namespace("github.com/user/fine@v1")

	a := &ucmocks.MockArrow{
		ListFn: func(_ context.Context, _ *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{
				{Namespace: "github.com/user/broken", Versions: []models.VersionView{{Namespace: broken}}},
				{Namespace: "github.com/user/fine", Versions: []models.VersionView{{Namespace: fine}}},
			}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			if ns == broken {
				return nil, assert.AnError
			}
			return &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateRunning}, nil
		},
	}
	uc := newUC(a, rt, &ucmocks.MockGraph{})

	got, err := uc.ListRuntimes(context.Background())

	require.NoError(t, err, "a single bad aggregate must not fail the listing")
	require.Len(t, got, 2)
	assert.Equal(t, domain.ArrowStateAbsent, got[0].State, "the unreadable one reports absent")
	assert.Equal(t, domain.ArrowStateRunning, got[1].State, "the readable one is unaffected")
}
