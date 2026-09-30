package deps

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func TestRuntimeUninstall_DependentsGuard(t *testing.T) {
	g := &mocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) {
			return true, nil
		},
	}
	rt := &mocks.MockRuntime{}
	uc := newUC(&mocks.MockArrow{}, rt, g)

	err := uc.Uninstall(context.Background(), "test/arrow@v1", nil)

	if !errors.Is(err, apperrors.ErrDependentsExist) {
		t.Fatalf("expected ErrDependentsExist, got %v", err)
	}
}

func TestRuntimeUninstall_SuccessWhenNoDependents(t *testing.T) {
	beginCalled := false
	g := &mocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) {
			return false, nil
		},
	}
	rt := &mocks.MockRuntime{
		BeginUninstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, g)

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

	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, nil
		},
	}
	rt := &mocks.MockRuntime{
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
	uc := newUC(&mocks.MockArrow{}, rt, g)

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

	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return []domain.Namespace{stoppedRef}, nil
		},
	}
	rt := &mocks.MockRuntime{
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
	uc := newUC(&mocks.MockArrow{}, rt, g)

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

func TestRuntimeStop_DelegatesToRuntime(t *testing.T) {
	called := false
	rt := &mocks.MockRuntime{
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			called = true
			return nil
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.Stop(context.Background(), "test/arrow@v1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected runtime.BeginStop to be called")
	}
}

func TestRuntimeInstall_NotExists(t *testing.T) {
	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, nil },
	}
	uc := newUC(a, &mocks.MockRuntime{}, &mocks.MockGraph{})
	if _, err := uc.Install(context.Background(), "test/arrow@v1", nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRuntimeInstall_ExistsError(t *testing.T) {
	expected := errors.New("exists error")
	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, expected },
	}
	uc := newUC(a, &mocks.MockRuntime{}, &mocks.MockGraph{})
	if _, err := uc.Install(context.Background(), "test/arrow@v1", nil); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestRuntimeInstall_GraphResolveError(t *testing.T) {
	expected := errors.New("graph error")
	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
		GetFn:    func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return &domain.Arrow{}, nil },
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return nil, expected
		},
	}
	uc := newUC(a, &mocks.MockRuntime{}, g)
	if _, err := uc.Install(context.Background(), "test/arrow@v1", nil); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestRuntimeInstall_NoDeps_Success(t *testing.T) {
	beginCalled := false
	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
		GetFn:    func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return &domain.Arrow{}, nil },
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{}, nil
		},
	}
	rt := &mocks.MockRuntime{
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

func TestRuntimeInstall_ReadyArrow_IsIdempotent(t *testing.T) {
	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{}, nil
		},
	}
	rt := &mocks.MockRuntime{
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
	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{}, nil
		},
	}
	rt := &mocks.MockRuntime{
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
	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
		GetFn:    func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return &domain.Arrow{}, nil },
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{}, nil
		},
	}
	rt := &mocks.MockRuntime{
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

	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
		GetFn:    func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return &domain.Arrow{}, nil },
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, ns domain.Namespace) (models.Plan, error) {
			if ns == mainNs {
				return models.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
			}
			return models.Plan{}, nil
		},
	}
	rt := &mocks.MockRuntime{
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

func TestInstallOneDep_ReadyDep_NoWait(t *testing.T) {
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
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
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			ch := make(chan domainRuntime.ArrowRuntime, 1)
			ch <- result
			return ch, func() {}, nil
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
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
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			ch := make(chan domainRuntime.ArrowRuntime, 1)
			ch <- result
			return ch, func() {}, nil
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.installOneDep(context.Background(), "test/dep@v1"); err == nil {
		t.Fatal("expected error for failed install")
	}
}

func TestInstallOneDep_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			cancel()
			return domain.ArrowStateAbsent, nil
		},
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			return make(chan domainRuntime.ArrowRuntime), func() {}, nil
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.installOneDep(ctx, "test/dep@v1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestStartServiceDep_AlreadyRunning_NoOp(t *testing.T) {
	beginCalled := false
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateRunning, nil
		},
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.startServiceDep(context.Background(), "test/dep@v1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if beginCalled {
		t.Fatal("expected BeginExecution NOT to be called when already running")
	}
}

func TestStartServiceDep_NotRunning_StartsExecution(t *testing.T) {
	beginCalled := false
	rt := &mocks.MockRuntime{
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
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.startServiceDep(context.Background(), "test/dep@v1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !beginCalled {
		t.Fatal("expected BeginExecution to be called with MethodExecute")
	}
}

func TestRuntimeOnEnded_NilLastReturn_NoOp(t *testing.T) {
	stopCalled := false
	rt := &mocks.MockRuntime{
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			stopCalled = true
			return nil
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{Ref: "test/arrow@v1"})
	if stopCalled {
		t.Fatal("expected no side effects for nil LastReturn")
	}
}

func TestRuntimeOnEnded_MethodInstall_NoOp(t *testing.T) {
	stopCalled := false
	rt := &mocks.MockRuntime{
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			stopCalled = true
			return nil
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/arrow@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodInstall},
	})
	if stopCalled {
		t.Fatal("expected no Stop call for MethodInstall")
	}
}

func TestRuntimeOnUninstallEnded_DepAbsent_NoAction(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	stopCalled := false

	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, nil
		},
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			stopCalled = true
			return nil
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, g)
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

	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, nil
		},
	}
	rt := &mocks.MockRuntime{
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
	uc := newUC(&mocks.MockArrow{
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

	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, nil
		},
	}
	rt := &mocks.MockRuntime{
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
	uc := newUC(&mocks.MockArrow{
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

// An orphaned dependency stuck at Outdated (a version-drift badge, not a
// dependency-sync one) must still get uninstalled — this is the sibling of
// runtimeUsecase.syncDeps's own dependency-removal switch, which already
// merges ArrowStateReady and ArrowStateOutdated into the same case.
// onUninstallEnded's switch previously left ArrowStateOutdated in its
// explicit no-op list, silently leaking an orphaned, outdated dependency.
func TestRuntimeOnUninstallEnded_DepOutdated_Uninstalls(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	beginCalled := false

	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, nil
		},
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == depNs {
				return domain.ArrowStateOutdated, nil
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
	uc := newUC(&mocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{UserInstalled: false}, nil
		},
	}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodUninstall},
	})
	if !beginCalled {
		t.Fatal("expected BeginUninstall for outdated dep")
	}
}

func TestRuntimeOnUninstallEnded_DepHasOtherRunningParent_NoAction(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	otherParent := domain.Namespace("test/other@v1")
	stopCalled := false

	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return []domain.Namespace{otherParent}, nil
		},
	}
	rt := &mocks.MockRuntime{
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
	uc := newUC(&mocks.MockArrow{}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodUninstall},
	})
	if stopCalled {
		t.Fatal("expected no Stop when dep still has other running parents")
	}
}

func TestMaybeAutoUninstallStopped_UserInstalled_NoOp(t *testing.T) {
	beginCalled := false
	a := &mocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{UserInstalled: true}, nil
		},
	}
	rt := &mocks.MockRuntime{
		BeginUninstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	newUC(a, rt, &mocks.MockGraph{}).maybeAutoUninstallStopped(context.Background(), "test/arrow@v1")
	if beginCalled {
		t.Fatal("expected no uninstall for user-installed arrow")
	}
}

func TestMaybeAutoUninstallStopped_GetArrowNil_NoOp(t *testing.T) {
	beginCalled := false
	a := &mocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return nil, nil },
	}
	rt := &mocks.MockRuntime{
		BeginUninstallFn: func(_ context.Context, _ domain.Namespace, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	newUC(a, rt, &mocks.MockGraph{}).maybeAutoUninstallStopped(context.Background(), "test/arrow@v1")
	if beginCalled {
		t.Fatal("expected no uninstall when arrow not found")
	}
}

func TestMaybeAutoUninstallStopped_ParentsStillRunning_NoOp(t *testing.T) {
	parentNs := domain.Namespace("test/parent@v1")
	beginCalled := false

	a := &mocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{UserInstalled: false}, nil
		},
	}
	g := &mocks.MockGraph{
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return []domain.Namespace{parentNs}, nil
		},
	}
	rt := &mocks.MockRuntime{
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
	a := &mocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{UserInstalled: false}, nil
		},
	}
	g := &mocks.MockGraph{
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, nil
		},
	}
	rt := &mocks.MockRuntime{
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

func TestRuntimeInstall_DepExistsError_ReturnsError(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	mainNs := domain.Namespace("test/main@v1")
	depErr := errors.New("dep check error")

	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, ns domain.Namespace) (bool, error) {
			if ns == mainNs {
				return true, nil
			}
			return false, depErr
		},
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return &domain.Arrow{}, nil },
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
	}
	uc := newUC(a, &mocks.MockRuntime{}, g)
	if _, err := uc.Install(context.Background(), mainNs, nil); !errors.Is(err, depErr) {
		t.Fatalf("expected dep check error, got %v", err)
	}
}

func TestRuntimeInstall_AddDependencyError_ReturnsError(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	mainNs := domain.Namespace("test/main@v1")
	resolveErr := errors.New("resolve error")

	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, ns domain.Namespace) (bool, error) {
			return ns == mainNs, nil
		},
		AddDependencyFn: func(context.Context, domain.Namespace) (domain.Namespace, error) {
			return "", resolveErr
		},
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
	}
	beginCalled := false
	rt := &mocks.MockRuntime{
		BeginInstallFn: func(context.Context, domain.Namespace, map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	uc := newUC(a, rt, g)
	_, err := uc.Install(context.Background(), mainNs, nil)
	require.ErrorIs(t, err, resolveErr)
	assert.False(t, beginCalled, "nothing installs when a dependency cannot be catalogued")
}

// A bare tools:/services: declaration stays bare in the plan; the row it
// installs is the identity AddDependency catalogues it as, and every later
// step keys off that identity.
func TestRuntimeInstall_DependencyIdentity_UsedForBeginInstall(t *testing.T) {
	testCases := []struct {
		name         string
		declared     domain.Namespace
		catalogued   bool
		identity     domain.Namespace
		wantAddedFor []domain.Namespace
	}{
		{
			name:         "bare declaration takes the identity it is added as",
			declared:     "github.com/rabbytesoftware/quiver.essentials/appimage-runtime",
			identity:     "github.com/rabbytesoftware/quiver.essentials/appimage-runtime@stable",
			wantAddedFor: []domain.Namespace{"github.com/rabbytesoftware/quiver.essentials/appimage-runtime"},
		},
		{
			name:         "a selector not catalogued yet is added as itself",
			declared:     "github.com/user/dep@v1.*",
			identity:     "github.com/user/dep@v1.*",
			wantAddedFor: []domain.Namespace{"github.com/user/dep@v1.*"},
		},
		{
			name:       "a catalogued selector is its own identity",
			declared:   "github.com/user/dep@v1.*",
			catalogued: true,
			identity:   "github.com/user/dep@v1.*",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mainNs := domain.Namespace("test/main@v1")
			var beganOn domain.Namespace
			var addedFor []domain.Namespace

			a := &mocks.MockArrow{
				ExistsFn: func(_ context.Context, ns domain.Namespace) (bool, error) {
					return ns == mainNs || (tc.catalogued && ns == tc.declared), nil
				},
				AddDependencyFn: func(_ context.Context, ns domain.Namespace) (domain.Namespace, error) {
					addedFor = append(addedFor, ns)
					return tc.identity, nil
				},
			}
			g := &mocks.MockGraph{
				ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
					return models.Plan{{Namespace: tc.declared, Type: domain.ToolDep}}, nil
				},
			}
			rt := &mocks.MockRuntime{
				GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
					return domain.ArrowStateAbsent, nil
				},
				BeginInstallFn: func(_ context.Context, ns domain.Namespace, _ map[string]string) error {
					if ns != mainNs {
						beganOn = ns
					}
					return nil
				},
				ListenEndedFn: endedWith(domainRuntime.ExecutionOutcomeSuccess),
			}
			uc := newUC(a, rt, g)

			_, err := uc.Install(context.Background(), mainNs, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.identity, beganOn)
			assert.Equal(t, tc.wantAddedFor, addedFor)
		})
	}
}

// endedWith answers ListenEnded with one finished execution of outcome.
func endedWith(
	outcome domainRuntime.ExecutionOutcome,
) func(context.Context, domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
	return func(context.Context, domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
		ch := make(chan domainRuntime.ArrowRuntime, 1)
		ch <- domainRuntime.ArrowRuntime{LastReturn: &domainRuntime.Return{Outcome: outcome}}
		return ch, func() {}, nil
	}
}

func TestRuntimeInstall_InstallOneDepError_ReturnsError(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	mainNs := domain.Namespace("test/main@v1")
	listenErr := errors.New("listen error")

	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
		GetFn:    func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return &domain.Arrow{}, nil },
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
	}
	rt := &mocks.MockRuntime{
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

	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
		GetFn:    func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return &domain.Arrow{}, nil },
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
	}
	rt := &mocks.MockRuntime{
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

func TestInstallOneDep_ListenEndedError_ReturnsError(t *testing.T) {
	listenErr := errors.New("listen error")
	rt := &mocks.MockRuntime{
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			return nil, nil, listenErr
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.installOneDep(context.Background(), "test/dep@v1"); !errors.Is(err, listenErr) {
		t.Fatalf("expected listen error, got %v", err)
	}
}

func TestStartServiceDep_GetStateError_ReturnsError(t *testing.T) {
	stateErr := errors.New("state error")
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return "", stateErr
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.startServiceDep(context.Background(), "test/dep@v1"); !errors.Is(err, stateErr) {
		t.Fatalf("expected state error, got %v", err)
	}
}

func TestStartServiceDep_BeginExecutionNonStateViolationError(t *testing.T) {
	execErr := errors.New("exec error")
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
			return execErr
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.startServiceDep(context.Background(), "test/dep@v1"); !errors.Is(err, execErr) {
		t.Fatalf("expected execErr, got %v", err)
	}
}

func TestStartServiceDep_BeginExecutionStateViolation_NoError(t *testing.T) {
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
			return apperrors.ErrStateViolation
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.startServiceDep(context.Background(), "test/dep@v1"); err != nil {
		t.Fatalf("expected no error for state violation, got %v", err)
	}
}

func TestRuntimeSyncDeps_GetRuntimeError_ReturnsError(t *testing.T) {
	getRtErr := errors.New("get runtime error")
	rt := &mocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return nil, getRtErr
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.syncDeps(context.Background(), "test/arrow@v1"); !errors.Is(err, getRtErr) {
		t.Fatalf("expected getRtErr, got %v", err)
	}
}

func TestRuntimeOnStopEnded_GetDependentsError_Continues(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	stopCalled := false

	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, errors.New("db error")
		},
	}
	rt := &mocks.MockRuntime{
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
	uc := newUC(&mocks.MockArrow{
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

func TestRuntimeOnUninstallEnded_GetDependentsError_Continues(t *testing.T) {
	depNs := domain.Namespace("test/dep@v1")
	stopCalled := false

	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, errors.New("db error")
		},
	}
	rt := &mocks.MockRuntime{
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
	uc := newUC(&mocks.MockArrow{}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodUninstall},
	})
	if stopCalled {
		t.Fatal("expected no Stop when GetDependents fails")
	}
}

func TestCountRunning_MixedStates(t *testing.T) {
	rt := &mocks.MockRuntime{
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
	n := countRunning(context.Background(), nil, (&mocks.MockRuntime{}).GetState)
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestRuntimeSyncDeps_RuntimeNil_StateViolation(t *testing.T) {
	rt := &mocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return nil, nil
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.syncDeps(context.Background(), "test/arrow@v1"); !errors.Is(err, apperrors.ErrStateViolation) {
		t.Fatalf("expected ErrStateViolation, got %v", err)
	}
}

func TestRuntimeSyncDeps_NoPendingSync_Clears(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")

	rt := &mocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:            ns,
				State:          domain.ArrowStateOutdated,
				PendingDepSync: nil,
			}, nil
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.syncDeps(context.Background(), ns); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRuntimeSyncDeps_WithAddedDep_AlreadyExists(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")

	rt := &mocks.MockRuntime{
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
	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
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

	rt := &mocks.MockRuntime{
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
	a := &mocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{UserInstalled: false}, nil
		},
	}
	g := &mocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) {
			return false, nil
		},
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{}, nil
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

func TestInstallOneDep_GetStateError_ReturnsError(t *testing.T) {
	stateErr := errors.New("state error")
	rt := &mocks.MockRuntime{
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			return make(chan domainRuntime.ArrowRuntime, 1), func() {}, nil
		},
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return "", stateErr
		},
	}
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.installOneDep(context.Background(), "test/dep@v1"); !errors.Is(err, stateErr) {
		t.Fatalf("expected stateErr, got %v", err)
	}
}

func TestInstallOneDep_BeginExecutionNonStateViolation_ReturnsError(t *testing.T) {
	beErr := errors.New("begin execution error")
	rt := &mocks.MockRuntime{
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
	uc := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{})
	if err := uc.installOneDep(context.Background(), "test/dep@v1"); !errors.Is(err, beErr) {
		t.Fatalf("expected beErr, got %v", err)
	}
}

func TestRuntimeUninstall_HasDependentsError_ReturnsError(t *testing.T) {
	depsErr := errors.New("deps error")
	g := &mocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) {
			return false, depsErr
		},
	}
	uc := newUC(&mocks.MockArrow{}, &mocks.MockRuntime{}, g)
	if err := uc.Uninstall(context.Background(), "test/arrow@v1", nil); !errors.Is(err, depsErr) {
		t.Fatalf("expected depsErr, got %v", err)
	}
}

func TestRuntimeSyncDeps_AddedDep_ExistsError_ReturnsError(t *testing.T) {
	existsErr := errors.New("exists error")
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	rt := &mocks.MockRuntime{
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
	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, existsErr },
	}
	uc := newUC(a, rt, &mocks.MockGraph{})
	if err := uc.syncDeps(context.Background(), ns); !errors.Is(err, existsErr) {
		t.Fatalf("expected existsErr, got %v", err)
	}
}

func TestRuntimeSyncDeps_AddedDep_AddDependencyError_ReturnsError(t *testing.T) {
	resolveErr := errors.New("resolve error")
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	rt := &mocks.MockRuntime{
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
	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, nil },
		AddDependencyFn: func(context.Context, domain.Namespace) (domain.Namespace, error) {
			return "", resolveErr
		},
	}
	uc := newUC(a, rt, &mocks.MockGraph{})
	if err := uc.syncDeps(context.Background(), ns); !errors.Is(err, resolveErr) {
		t.Fatalf("expected resolveErr, got %v", err)
	}
}

func TestRuntimeSyncDeps_AddedDep_InstallOneDepError_ReturnsError(t *testing.T) {
	listenErr := errors.New("listen error")
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	rt := &mocks.MockRuntime{
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
	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	uc := newUC(a, rt, &mocks.MockGraph{})
	if err := uc.syncDeps(context.Background(), ns); !errors.Is(err, listenErr) {
		t.Fatalf("expected listenErr, got %v", err)
	}
}

func TestRuntimeSyncDeps_AddedServiceDep_StartError_ReturnsError(t *testing.T) {
	startErr := errors.New("start error")
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	rt := &mocks.MockRuntime{
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
	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
	}
	uc := newUC(a, rt, g)
	if err := uc.syncDeps(context.Background(), ns); !errors.Is(err, startErr) {
		t.Fatalf("expected startErr, got %v", err)
	}
}

func TestRuntimeSyncDeps_RemovedDep_GetArrowError_Skips(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	beginCalled := false
	rt := &mocks.MockRuntime{
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
	a := &mocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return nil, errors.New("get error")
		},
	}
	uc := newUC(a, rt, &mocks.MockGraph{})
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
	rt := &mocks.MockRuntime{
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
	a := &mocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: depNs, UserInstalled: false}, nil
		},
	}
	g := &mocks.MockGraph{
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
	rt := &mocks.MockRuntime{
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
	a := &mocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: depNs, UserInstalled: false}, nil
		},
	}
	g := &mocks.MockGraph{
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
	rt := &mocks.MockRuntime{
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
	a := &mocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: depNs, UserInstalled: false}, nil
		},
	}
	g := &mocks.MockGraph{
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

func TestRuntimeOnStopEnded_GraphResolveError_NoOp(t *testing.T) {
	stopCalled := false
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return nil, errors.New("resolve error")
		},
	}
	rt := &mocks.MockRuntime{
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error { stopCalled = true; return nil },
	}
	uc := newUC(&mocks.MockArrow{
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
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) { return nil, nil },
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateRunning, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error { stopCalled = true; return nil },
	}
	uc := newUC(&mocks.MockArrow{
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
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) { return nil, nil },
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return "", errors.New("state error")
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error { stopCalled = true; return nil },
	}
	uc := newUC(&mocks.MockArrow{
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
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) { return nil, nil },
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error { stopCalled = true; return nil },
	}
	uc := newUC(&mocks.MockArrow{
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
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ServiceDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, ns domain.Namespace) ([]domain.Namespace, error) {
			if ns == depNs {
				return []domain.Namespace{"test/app@v1", otherParentNs}, nil
			}
			return nil, nil
		},
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == depNs || ns == otherParentNs {
				return domain.ArrowStateRunning, nil
			}
			return domain.ArrowStateAbsent, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error { stopCalled = true; return nil },
	}
	uc := newUC(&mocks.MockArrow{
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

func TestMaybeAutoUninstallStopped_GetDependentsError_NoOp(t *testing.T) {
	ns := domain.Namespace("test/dep@v1")
	beginCalled := false
	g := &mocks.MockGraph{
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, errors.New("get dependents error")
		},
	}
	rt := &mocks.MockRuntime{
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
			beginCalled = true
			return nil
		},
	}
	uc := newUC(&mocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns, UserInstalled: false}, nil
		},
	}, rt, g)
	uc.maybeAutoUninstallStopped(context.Background(), ns)
	if beginCalled {
		t.Fatal("expected no BeginExecution when GetDependents fails")
	}
}

func TestRuntimeOnUninstallEnded_GraphResolveError_NoOp(t *testing.T) {
	stopCalled := false
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return nil, errors.New("resolve error")
		},
	}
	rt := &mocks.MockRuntime{
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error { stopCalled = true; return nil },
	}
	uc := newUC(&mocks.MockArrow{}, rt, g)
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
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: depNs, Type: domain.ToolDep}}, nil
		},
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) { return nil, nil },
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return "", errors.New("state error")
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error { stopCalled = true; return nil },
	}
	uc := newUC(&mocks.MockArrow{}, rt, g)
	uc.onRuntimeEnded(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        "test/app@v1",
		LastReturn: &domainRuntime.Return{Method: domain.MethodUninstall},
	})
	if stopCalled {
		t.Fatal("expected no Stop when GetState fails")
	}
}

// The catalog only ever stores versioned namespaces, but every command accepts
// a bare one: `arrow add <bare>` resolves the ref and succeeds, so `install
// <bare>` reporting "not found" makes the namespace that worked a moment ago
// unusable.
func TestRuntimeInstall_ResolvesBareNamespaceToTheCataloguedRef(t *testing.T) {
	bare := domain.Namespace("github.com/user/app")
	versioned := domain.Namespace("github.com/user/app@main")

	var begunOn domain.Namespace

	a := &mocks.MockArrow{
		ResolveCataloguedFn: func(_ context.Context, ns domain.Namespace) (domain.Namespace, error) {
			if ns == bare {
				return versioned, nil
			}
			return ns, nil
		},
		ExistsFn: func(_ context.Context, ns domain.Namespace) (bool, error) {
			return ns == versioned, nil
		},
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return &domain.Arrow{}, nil },
	}

	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
		BeginInstallFn: func(_ context.Context, ns domain.Namespace, _ map[string]string) error {
			begunOn = ns
			return nil
		},
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) { return nil, nil },
	}
	uc := newUC(a, rt, g)

	started, err := uc.Install(context.Background(), bare, nil)

	require.NoError(t, err)
	assert.True(t, started)
	assert.Equal(t, versioned, begunOn, "the install must run against the catalogued ref")
}

// A namespace that is genuinely absent from the catalog still reports not
// found; resolution must not invent one.
func TestRuntimeInstall_UnknownNamespaceStillNotFound(t *testing.T) {
	a := &mocks.MockArrow{
		ResolveCataloguedFn: func(_ context.Context, _ domain.Namespace) (domain.Namespace, error) {
			return "", apperrors.ErrNotFound
		},
	}
	uc := newUC(a, &mocks.MockRuntime{}, &mocks.MockGraph{})

	_, err := uc.Install(context.Background(), "github.com/user/nope", nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

// An install never swaps identity: a row that is behind installs what it
// resolved to, and catches up through an update.
// An install of a row nothing is installed from advances it to the release
// ahead in place: the identity it begins on is the row's own.
func TestRuntimeInstall_OutdatedRow_InstallsItsOwnIdentity(t *testing.T) {
	ns := domain.Namespace("github.com/char2cs/crowbar@nightly")
	var begunOn domain.Namespace
	var advancedTo []domain.Available

	a := &mocks.MockArrow{
		ExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return true, nil },
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns, Available: &domain.Available{Ref: "nightly", Commit: "c2"}}, nil
		},
		AdvanceFn: func(_ context.Context, advanced domain.Namespace, target domain.Available) error {
			assert.Equal(t, ns, advanced)
			advancedTo = append(advancedTo, target)
			return nil
		},
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (models.Plan, error) { return nil, nil },
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
		BeginInstallFn: func(_ context.Context, got domain.Namespace, _ map[string]string) error {
			begunOn = got
			return nil
		},
	}
	uc := newUC(a, rt, g)

	started, err := uc.Install(context.Background(), ns, nil)

	require.NoError(t, err)
	assert.True(t, started)
	assert.Equal(t, ns, begunOn)
	assert.Equal(t, []domain.Available{{Ref: "nightly", Commit: "c2"}}, advancedTo)
}

func TestRuntimeStop_ResolvesBareNamespace(t *testing.T) {
	bare := domain.Namespace("github.com/user/app")
	versioned := domain.Namespace("github.com/user/app@main")

	var stoppedOn domain.Namespace

	a := &mocks.MockArrow{
		ResolveCataloguedFn: func(_ context.Context, _ domain.Namespace) (domain.Namespace, error) {
			return versioned, nil
		},
	}
	rt := &mocks.MockRuntime{
		BeginStopFn: func(_ context.Context, ns domain.Namespace) error {
			stoppedOn = ns
			return nil
		},
	}
	uc := newUC(a, rt, &mocks.MockGraph{})

	require.NoError(t, uc.Stop(context.Background(), bare))
	assert.Equal(t, versioned, stoppedOn)
}

func TestRuntimeInstall_RuntimeFailures(t *testing.T) {
	boom := errors.New("boom")

	testCases := []struct {
		name     string
		stateErr error
		beginErr error
	}{
		{name: "state cannot be read", stateErr: boom},
		{name: "install cannot begin", beginErr: boom},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ns := domain.Namespace("github.com/user/app@stable")
			a := &mocks.MockArrow{
				ResolveCataloguedFn: func(_ context.Context, got domain.Namespace) (domain.Namespace, error) { return got, nil },
				ExistsFn:            func(context.Context, domain.Namespace) (bool, error) { return true, nil },
			}
			rt := &mocks.MockRuntime{
				GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
					return domain.ArrowStateAbsent, tc.stateErr
				},
				BeginInstallFn: func(context.Context, domain.Namespace, map[string]string) error { return tc.beginErr },
			}

			_, err := newUC(a, rt, &mocks.MockGraph{}).Install(context.Background(), ns, nil)

			require.ErrorIs(t, err, boom)
		})
	}
}

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
			rt := &mocks.MockRuntime{
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

			err := newUC(&mocks.MockArrow{}, rt, &mocks.MockGraph{}).deps.StopIfRunning(ctx, rollingRow)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantStop, stopped)
		})
	}
}
