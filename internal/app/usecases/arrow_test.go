package usecases

import (
	"context"
	"errors"
	"testing"

	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/graph"
	ucmocks "github.com/rabbytesoftware/quiver/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

// --- tests ---

func TestArrowRemove_StateViolationGuard(t *testing.T) {
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateRunning, nil
		},
	}

	uc := NewArrowUsecase(&ucmocks.MockArrow{}, &ucmocks.MockGraph{}, rt)
	err := uc.Remove(context.Background(), "test/arrow@v1")

	if !errors.Is(err, apperrors.ErrStateViolation) {
		t.Fatalf("expected ErrStateViolation, got %v", err)
	}
}

func TestArrowRemove_SuccessWhenAbsent(t *testing.T) {
	removeCalled := false

	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
	}
	a := &ucmocks.MockArrow{
		RemoveFn: func(_ context.Context, _ domain.Namespace) error {
			removeCalled = true
			return nil
		},
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, rt)
	err := uc.Remove(context.Background(), "test/arrow@v1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !removeCalled {
		t.Fatal("expected arrow.Remove to be called")
	}
}

func TestArrowHasDependents_DelegatesToGraph(t *testing.T) {
	target := domain.Namespace("test/arrow@v1")
	exclude := domain.Namespace("test/other@v1")
	called := false

	g := &ucmocks.MockGraph{
		HasDependentsFn: func(_ context.Context, ns domain.Namespace, excludeNs domain.Namespace) (bool, error) {
			called = true
			if ns != target {
				t.Errorf("got ns=%q, want %q", ns, target)
			}
			if excludeNs != exclude {
				t.Errorf("got excludeNs=%q, want %q", excludeNs, exclude)
			}
			return true, nil
		},
	}

	uc := NewArrowUsecase(&ucmocks.MockArrow{}, g, &ucmocks.MockRuntime{})
	got, err := uc.HasDependents(context.Background(), target, exclude)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatal("expected HasDependents to return true")
	}
	if !called {
		t.Fatal("expected graph.HasDependents to be called")
	}
}

func TestArrowList_DelegatesToArrow(t *testing.T) {
	userInstalled := true
	returnedViews := []models.ArrowView{
		{Namespace: "test/arrow"},
	}

	a := &ucmocks.MockArrow{
		ListFn: func(_ context.Context, ui *bool) ([]models.ArrowView, error) {
			if ui == nil || *ui != userInstalled {
				t.Errorf("unexpected userInstalled param")
			}
			return returnedViews, nil
		},
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	dtos, err := uc.List(context.Background(), &userInstalled)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dtos) != 1 {
		t.Fatalf("expected 1 DTO, got %d", len(dtos))
	}
}

func TestArrowGet_DelegatesToArrow(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	returnedArrow := &domain.Arrow{Namespace: ns}

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, got domain.Namespace) (*domain.Arrow, error) {
			if got != ns {
				t.Errorf("got ns=%q, want %q", got, ns)
			}
			return returnedArrow, nil
		},
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	result, err := uc.Get(context.Background(), ns)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != returnedArrow {
		t.Fatal("expected returned arrow to match")
	}
}

func TestArrowAdd_Success(t *testing.T) {
	called := false
	a := &ucmocks.MockArrow{
		AddFn: func(_ context.Context, _ domain.Namespace) error {
			called = true
			return nil
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if err := uc.Add(context.Background(), "test/arrow@v1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected arrow.Add to be called")
	}
}

func TestArrowAdd_PropagatesError(t *testing.T) {
	expected := errors.New("add error")
	a := &ucmocks.MockArrow{
		AddFn: func(_ context.Context, _ domain.Namespace) error { return expected },
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if err := uc.Add(context.Background(), "test/arrow@v1"); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestArrowRemove_GetStateError(t *testing.T) {
	expected := errors.New("state error")
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return "", expected
		},
	}
	uc := NewArrowUsecase(&ucmocks.MockArrow{}, &ucmocks.MockGraph{}, rt)
	if err := uc.Remove(context.Background(), "test/arrow@v1"); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestArrowRemove_HasDependentsError(t *testing.T) {
	expected := errors.New("graph error")
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
	}
	g := &ucmocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _ domain.Namespace, _ domain.Namespace) (bool, error) {
			return false, expected
		},
	}
	uc := NewArrowUsecase(&ucmocks.MockArrow{}, g, rt)
	if err := uc.Remove(context.Background(), "test/arrow@v1"); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestArrowRemove_HasDependents_Blocked(t *testing.T) {
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
	}
	g := &ucmocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _ domain.Namespace, _ domain.Namespace) (bool, error) {
			return true, nil
		},
	}
	uc := NewArrowUsecase(&ucmocks.MockArrow{}, g, rt)
	if err := uc.Remove(context.Background(), "test/arrow@v1"); !errors.Is(err, apperrors.ErrDependentsExist) {
		t.Fatalf("expected ErrDependentsExist, got %v", err)
	}
}

func TestArrowList_PropagatesError(t *testing.T) {
	expected := errors.New("list error")
	a := &ucmocks.MockArrow{
		ListFn: func(_ context.Context, _ *bool) ([]models.ArrowView, error) {
			return nil, expected
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.List(context.Background(), nil); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestArrowUpdate_PropagatesGetError(t *testing.T) {
	expected := errors.New("get error")
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return nil, expected
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.Update(context.Background(), "test/arrow@v1", models.UpdateOptions{}); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestArrowUpdate_NoDrift_AppliesManifest(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	current := &domain.Arrow{Namespace: ns}
	newArrow := &domain.Arrow{Namespace: ns}
	updateCalled := false

	a := &ucmocks.MockArrow{
		GetFn:             func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ResolveManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return newArrow, nil },
		UpdateManifestFn: func(_ context.Context, _ domain.Namespace, _ *domain.Arrow) error {
			updateCalled = true
			return nil
		},
	}
	g := &ucmocks.MockGraph{
		DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff { return graph.DepDiff{} },
	}

	uc := NewArrowUsecase(a, g, &ucmocks.MockRuntime{})
	result, err := uc.Update(context.Background(), ns, models.UpdateOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updateCalled {
		t.Fatal("expected UpdateManifest to be called")
	}
	if len(result.AddedDeps) != 0 {
		t.Fatalf("expected no added deps, got %d", len(result.AddedDeps))
	}
}

func TestArrowUpdate_WithDrift_MarkOutdated(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	depNs := domain.Namespace("test/dep@v1")
	markCalled := false

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		ResolveManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		UpdateManifestFn: func(_ context.Context, _ domain.Namespace, _ *domain.Arrow) error { return nil },
	}
	g := &ucmocks.MockGraph{
		DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff {
			return graph.DepDiff{Added: []domain.DependencyEdge{{Namespace: depNs}}}
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		MarkOutdatedFn: func(_ context.Context, _ domain.Namespace, added, _ []domain.Namespace) error {
			markCalled = true
			return nil
		},
	}

	uc := NewArrowUsecase(a, g, rt)
	result, err := uc.Update(context.Background(), ns, models.UpdateOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !markCalled {
		t.Fatal("expected MarkOutdated to be called")
	}
	if len(result.AddedDeps) != 1 || result.AddedDeps[0] != depNs {
		t.Fatalf("unexpected AddedDeps: %v", result.AddedDeps)
	}
}

func TestArrowUpdate_UpgradeRef_SameRef(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns, InstalledConstraint: "^v1"}
	newArrow := &domain.Arrow{Namespace: ns}

	a := &ucmocks.MockArrow{
		GetFn:               func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ResolveConstraintFn: func(_ context.Context, _ domain.Namespace, _ string) (string, error) { return "v1.0.0", nil },
		ResolveManifestFn:   func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return newArrow, nil },
		UpdateManifestFn:    func(_ context.Context, _ domain.Namespace, _ *domain.Arrow) error { return nil },
	}
	g := &ucmocks.MockGraph{
		DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff { return graph.DepDiff{} },
	}

	uc := NewArrowUsecase(a, g, &ucmocks.MockRuntime{})
	if _, err := uc.Update(context.Background(), ns, models.UpdateOptions{UpgradeRef: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestArrowUpdate_UpgradeRef_NewRef(t *testing.T) {
	oldNs := domain.Namespace("test/arrow@v1.0.0")
	newNs := domain.Namespace("test/arrow@v1.1.0")
	current := &domain.Arrow{Namespace: oldNs, InstalledConstraint: "^v1"}
	newArrow := &domain.Arrow{Namespace: newNs}

	a := &ucmocks.MockArrow{
		GetFn:               func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ResolveConstraintFn: func(_ context.Context, _ domain.Namespace, _ string) (string, error) { return "v1.1.0", nil },
		UpgradeVersionFn: func(_ context.Context, _, _ domain.Namespace, _ string, _ bool) (*domain.Arrow, error) {
			return newArrow, nil
		},
	}
	g := &ucmocks.MockGraph{
		DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff { return graph.DepDiff{} },
	}
	rt := &ucmocks.MockRuntime{
		RuntimeExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, nil },
	}

	uc := NewArrowUsecase(a, g, rt)
	if _, err := uc.Update(context.Background(), oldNs, models.UpdateOptions{UpgradeRef: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestArrowUpdate_UpgradeRef_ResolveConstraintError(t *testing.T) {
	expected := errors.New("constraint error")
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns, InstalledConstraint: "^v1"}

	a := &ucmocks.MockArrow{
		GetFn:               func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ResolveConstraintFn: func(_ context.Context, _ domain.Namespace, _ string) (string, error) { return "", expected },
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.Update(context.Background(), ns, models.UpdateOptions{UpgradeRef: true}); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestArrowGetDetail_WithRuntime(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	detail := &models.ArrowDetailView{Metadata: domain.Arrow{Namespace: ns}}
	exec := &domainRuntime.Execution{Method: domain.MethodInstall}
	rt := &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady, Execution: exec}

	a := &ucmocks.MockArrow{
		GetDetailFn: func(_ context.Context, _ domain.Namespace) (*models.ArrowDetailView, error) {
			return detail, nil
		},
	}
	mockRT := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return rt, nil
		},
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, mockRT)
	dto, err := uc.GetDetail(context.Background(), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dto == nil {
		t.Fatal("expected non-nil DTO")
	}
}

func TestArrowGetDetail_WithoutRuntime(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	detail := &models.ArrowDetailView{Metadata: domain.Arrow{Namespace: ns}}

	a := &ucmocks.MockArrow{
		GetDetailFn: func(_ context.Context, _ domain.Namespace) (*models.ArrowDetailView, error) {
			return detail, nil
		},
	}
	mockRT := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return nil, nil
		},
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, mockRT)
	dto, err := uc.GetDetail(context.Background(), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dto == nil {
		t.Fatal("expected non-nil DTO")
	}
}

func TestArrowGetDetail_GetDetailError(t *testing.T) {
	expected := errors.New("detail error")
	a := &ucmocks.MockArrow{
		GetDetailFn: func(_ context.Context, _ domain.Namespace) (*models.ArrowDetailView, error) {
			return nil, expected
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.GetDetail(context.Background(), "test/arrow@v1"); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestArrowGetManifest_WithRef_Error(t *testing.T) {
	uc := NewArrowUsecase(&ucmocks.MockArrow{}, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.GetManifest(context.Background(), "test/arrow@v1"); !errors.Is(err, apperrors.ErrInvalidNamespace) {
		t.Fatalf("expected ErrInvalidNamespace, got %v", err)
	}
}

func TestArrowGetManifest_Success(t *testing.T) {
	ns := domain.Namespace("test/arrow")
	a := &ucmocks.MockArrow{
		ResolveManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	dto, err := uc.GetManifest(context.Background(), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dto == nil {
		t.Fatal("expected non-nil DTO")
	}
}

func TestArrowSeed_DelegatesToArrow(t *testing.T) {
	called := false
	a := &ucmocks.MockArrow{
		SeedFn: func(_ context.Context, _ domain.Namespace, _ []byte) error {
			called = true
			return nil
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if err := uc.Seed(context.Background(), "test/arrow@v1", []byte("data")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected arrow.Seed to be called")
	}
}

func TestArrowValidateManifest_DelegatesToArrow(t *testing.T) {
	result := &models.ValidationResult{}
	a := &ucmocks.MockArrow{
		ValidateManifestFn: func(_ context.Context, _ []byte) (*models.ValidationResult, error) {
			return result, nil
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	got, err := uc.ValidateManifest(context.Background(), []byte("manifest"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != result {
		t.Fatal("expected returned result to match")
	}
}

// ─── Update: additional error paths ──────────────────────────────────────────

func TestArrowUpdate_ResolveManifestError_ReturnsError(t *testing.T) {
	resolveErr := errors.New("resolve error")
	ns := domain.Namespace("test/arrow@v1")
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		ResolveManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return nil, resolveErr
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.Update(context.Background(), ns, models.UpdateOptions{}); !errors.Is(err, resolveErr) {
		t.Fatalf("expected resolveErr, got %v", err)
	}
}

func TestArrowUpdate_UpdateManifestError_ReturnsError(t *testing.T) {
	updateErr := errors.New("update error")
	ns := domain.Namespace("test/arrow@v1")
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		ResolveManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		UpdateManifestFn: func(_ context.Context, _ domain.Namespace, _ *domain.Arrow) error {
			return updateErr
		},
	}
	g := &ucmocks.MockGraph{
		DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff { return graph.DepDiff{} },
	}
	uc := NewArrowUsecase(a, g, &ucmocks.MockRuntime{})
	if _, err := uc.Update(context.Background(), ns, models.UpdateOptions{}); !errors.Is(err, updateErr) {
		t.Fatalf("expected updateErr, got %v", err)
	}
}

// ─── upgradeRef: additional error paths ──────────────────────────────────────

func TestArrowUpdate_UpgradeRef_SameRef_ResolveManifestError(t *testing.T) {
	resolveErr := errors.New("resolve error")
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns, InstalledConstraint: "^v1"}
	a := &ucmocks.MockArrow{
		GetFn:               func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ResolveConstraintFn: func(_ context.Context, _ domain.Namespace, _ string) (string, error) { return "v1.0.0", nil },
		ResolveManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return nil, resolveErr
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.Update(context.Background(), ns, models.UpdateOptions{UpgradeRef: true}); !errors.Is(err, resolveErr) {
		t.Fatalf("expected resolveErr, got %v", err)
	}
}

func TestArrowUpdate_UpgradeRef_SameRef_UpdateManifestError(t *testing.T) {
	updateErr := errors.New("update error")
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns, InstalledConstraint: "^v1"}
	a := &ucmocks.MockArrow{
		GetFn:               func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ResolveConstraintFn: func(_ context.Context, _ domain.Namespace, _ string) (string, error) { return "v1.0.0", nil },
		ResolveManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		UpdateManifestFn: func(_ context.Context, _ domain.Namespace, _ *domain.Arrow) error {
			return updateErr
		},
	}
	g := &ucmocks.MockGraph{
		DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff { return graph.DepDiff{} },
	}
	uc := NewArrowUsecase(a, g, &ucmocks.MockRuntime{})
	if _, err := uc.Update(context.Background(), ns, models.UpdateOptions{UpgradeRef: true}); !errors.Is(err, updateErr) {
		t.Fatalf("expected updateErr, got %v", err)
	}
}

func TestArrowUpdate_UpgradeRef_NewRef_UpgradeVersionError(t *testing.T) {
	upgradeErr := errors.New("upgrade error")
	oldNs := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: oldNs, InstalledConstraint: "^v1"}
	a := &ucmocks.MockArrow{
		GetFn:               func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ResolveConstraintFn: func(_ context.Context, _ domain.Namespace, _ string) (string, error) { return "v1.1.0", nil },
		UpgradeVersionFn: func(_ context.Context, _, _ domain.Namespace, _ string, _ bool) (*domain.Arrow, error) {
			return nil, upgradeErr
		},
	}
	rt := &ucmocks.MockRuntime{
		RuntimeExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, nil },
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, rt)
	if _, err := uc.Update(context.Background(), oldNs, models.UpdateOptions{UpgradeRef: true}); !errors.Is(err, upgradeErr) {
		t.Fatalf("expected upgradeErr, got %v", err)
	}
}

// ─── GetManifest: ResolveManifest error path ─────────────────────────────────

func TestArrowGetManifest_ResolveManifestError(t *testing.T) {
	resolveErr := errors.New("resolve error")
	a := &ucmocks.MockArrow{
		ResolveManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return nil, resolveErr
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.GetManifest(context.Background(), "test/arrow"); !errors.Is(err, resolveErr) {
		t.Fatalf("expected resolveErr, got %v", err)
	}
}
