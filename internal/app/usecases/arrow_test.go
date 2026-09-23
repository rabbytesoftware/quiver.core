package usecases

import (
	"context"
	"errors"
	"testing"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
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
		HasDependentsFn: func(_ context.Context, ns, excludeNs domain.Namespace) (bool, error) {
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

func TestArrowGetDependents_DelegatesToGraph(t *testing.T) {
	target := domain.Namespace("test/arrow@v1")
	want := []domain.Namespace{"test/parent@v1"}
	called := false

	g := &ucmocks.MockGraph{
		GetDependentsFn: func(_ context.Context, ns domain.Namespace) ([]domain.Namespace, error) {
			called = true
			if ns != target {
				t.Errorf("got ns=%q, want %q", ns, target)
			}
			return want, nil
		},
	}

	uc := NewArrowUsecase(&ucmocks.MockArrow{}, g, &ucmocks.MockRuntime{})
	got, err := uc.GetDependents(context.Background(), target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("got %v, want %v", got, want)
	}
	if !called {
		t.Fatal("expected graph.GetDependents to be called")
	}
}

func TestArrowGetDependents_PropagatesError(t *testing.T) {
	wantErr := errors.New("edge store unavailable")
	g := &ucmocks.MockGraph{
		GetDependentsFn: func(_ context.Context, _ domain.Namespace) ([]domain.Namespace, error) {
			return nil, wantErr
		},
	}

	uc := NewArrowUsecase(&ucmocks.MockArrow{}, g, &ucmocks.MockRuntime{})
	_, err := uc.GetDependents(context.Background(), "test/arrow@v1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestArrowGetDependencies_DelegatesToGraph(t *testing.T) {
	target := domain.Namespace("test/arrow@v1")
	want := graph.Plan{{Namespace: "test/dep@v1", Type: domain.ToolDep}}
	called := false

	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, ns domain.Namespace) (graph.Plan, error) {
			called = true
			if ns != target {
				t.Errorf("got ns=%q, want %q", ns, target)
			}
			return want, nil
		},
	}

	uc := NewArrowUsecase(&ucmocks.MockArrow{}, g, &ucmocks.MockRuntime{})
	got, err := uc.GetDependencies(context.Background(), target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Namespace != want[0].Namespace || got[0].Type != want[0].Type {
		t.Fatalf("got %v, want %v", got, want)
	}
	if !called {
		t.Fatal("expected graph.Resolve to be called")
	}
}

func TestArrowGetDependencies_PropagatesError(t *testing.T) {
	wantErr := errors.New("cycle detected")
	g := &ucmocks.MockGraph{
		ResolveFn: func(_ context.Context, _ domain.Namespace) (graph.Plan, error) {
			return nil, wantErr
		},
	}

	uc := NewArrowUsecase(&ucmocks.MockArrow{}, g, &ucmocks.MockRuntime{})
	_, err := uc.GetDependencies(context.Background(), "test/arrow@v1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
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
		AddFn: func(_ context.Context, _ domain.Namespace, _ models.AddOptions) error {
			called = true
			return nil
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if err := uc.Add(context.Background(), "test/arrow@v1", models.AddOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected arrow.Add to be called")
	}
}

func TestArrowAdd_PropagatesError(t *testing.T) {
	expected := errors.New("add error")
	a := &ucmocks.MockArrow{
		AddFn: func(_ context.Context, _ domain.Namespace, _ models.AddOptions) error { return expected },
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if err := uc.Add(context.Background(), "test/arrow@v1", models.AddOptions{}); !errors.Is(err, expected) {
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
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) {
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
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) {
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

// TestArrowUpdate_ResolveCataloguedError_ReturnsError covers Update's first
// step, distinct from TestArrowUpdate_PropagatesGetError which covers the
// second (u.arrow.Get) — both must independently propagate.
func TestArrowUpdate_ResolveCataloguedError_ReturnsError(t *testing.T) {
	expected := errors.New("resolve catalogued error")
	a := &ucmocks.MockArrow{
		ResolveCataloguedFn: func(_ context.Context, _ domain.Namespace) (domain.Namespace, error) {
			return "", expected
		},
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			t.Fatal("Get must not be called when ResolveCatalogued fails")
			return nil, nil
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.Update(context.Background(), "test/arrow@v1", models.UpdateOptions{}); !errors.Is(err, expected) {
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

// An arrow already Outdated from a version-drift check (state == Outdated,
// not Ready) must still record a dependency-graph change discovered by a
// manifest update — otherwise the signal is silently dropped and a later
// BeginUpdate proceeds without installing the newly-added dependency.
func TestArrowUpdate_WithDrift_OutdatedState_StillMarksOutdated(t *testing.T) {
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
			return domain.ArrowStateOutdated, nil
		},
		MarkOutdatedFn: func(_ context.Context, _ domain.Namespace, added, _ []domain.Namespace) error {
			markCalled = true
			return nil
		},
	}

	uc := NewArrowUsecase(a, g, rt)
	_, err := uc.Update(context.Background(), ns, models.UpdateOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !markCalled {
		t.Fatal("expected MarkOutdated to be called even though state was already Outdated")
	}
}

func TestArrowUpdate_UpgradeRef_SameRef(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns, InstalledConstraint: "^v1"}
	newArrow := &domain.Arrow{Namespace: ns}

	a := &ucmocks.MockArrow{
		GetFn:               func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ResolveTrackedRefFn: func(_ context.Context, _ domain.Arrow) (string, error) { return "v1.0.0", nil },
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
		ResolveTrackedRefFn: func(_ context.Context, _ domain.Arrow) (string, error) { return "v1.1.0", nil },
		UpgradeVersionFn: func(_ context.Context, _, _ domain.Namespace, _, _ string, _, _, _ bool) (*domain.Arrow, error) {
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

// TestArrowUpdate_UpgradeRef_PassesUserInstalledThrough guards the regression
// a review caught: upgradeRef must forward the pre-upgrade arrow's own
// UserInstalled flag to UpgradeVersion, not silently drop it — the current
// arrow already carries the fact in scope, so there is no reason to lose it.
func TestArrowUpdate_UpgradeRef_PassesUserInstalledThrough(t *testing.T) {
	oldNs := domain.Namespace("test/arrow@v1.0.0")
	newNs := domain.Namespace("test/arrow@v1.1.0")
	current := &domain.Arrow{Namespace: oldNs, InstalledConstraint: "^v1", UserInstalled: true}
	newArrow := &domain.Arrow{Namespace: newNs}
	var gotUserInstalled bool

	a := &ucmocks.MockArrow{
		GetFn:               func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ResolveTrackedRefFn: func(_ context.Context, _ domain.Arrow) (string, error) { return "v1.1.0", nil },
		UpgradeVersionFn: func(_ context.Context, _, _ domain.Namespace, _, _ string, _, _ bool, userInstalled bool) (*domain.Arrow, error) {
			gotUserInstalled = userInstalled
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
	if !gotUserInstalled {
		t.Fatal("expected the pre-upgrade arrow's UserInstalled to carry through to UpgradeVersion")
	}
}

func TestArrowUpdate_UpgradeRef_ResolveTrackedRefError(t *testing.T) {
	expected := errors.New("tracked ref error")
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns, InstalledConstraint: "^v1"}

	a := &ucmocks.MockArrow{
		GetFn:               func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ResolveTrackedRefFn: func(_ context.Context, _ domain.Arrow) (string, error) { return "", expected },
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.Update(context.Background(), ns, models.UpdateOptions{UpgradeRef: true}); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

// TestArrowUpdate_UpgradeRef_ChannelTrackedNoConstraint_TakesUpgradePath
// pins the widened Update gate this round's fix exists for: a
// channel-tracked arrow with NO InstalledConstraint (the normal shape once
// an arrow's channel has been set) must still reach upgradeRef on an
// UpgradeRef request — before this fix it silently fell through to
// plainRefresh instead, a no-op re-fetch at the same ref, leaving the
// "Update available" button do nothing.
func TestArrowUpdate_UpgradeRef_ChannelTrackedNoConstraint_TakesUpgradePath(t *testing.T) {
	oldNs := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: oldNs, Channel: "beta"}
	var resolvedTrackedRef bool

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ResolveTrackedRefFn: func(_ context.Context, arrow domain.Arrow) (string, error) {
			resolvedTrackedRef = true
			if arrow.Channel != "beta" {
				t.Fatalf("ResolveTrackedRef called with Channel=%q, want %q", arrow.Channel, "beta")
			}
			return "v1.1.0-beta.1", nil
		},
		UpgradeVersionFn: func(_ context.Context, _, newArg domain.Namespace, _, _ string, _, _, _ bool) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: newArg}, nil
		},
		SetChannelFn: func(_ context.Context, _ domain.Namespace, _ string, _ string) error { return nil },
	}
	g := &ucmocks.MockGraph{DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff { return graph.DepDiff{} }}
	rt := &ucmocks.MockRuntime{
		RuntimeExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, nil },
	}

	uc := NewArrowUsecase(a, g, rt)
	if _, err := uc.Update(context.Background(), oldNs, models.UpdateOptions{UpgradeRef: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resolvedTrackedRef {
		t.Fatal("expected upgradeRef to run and resolve the tracked ref, not fall through to plainRefresh")
	}
}

// TestArrowUpdate_UpgradeRef_NoConstraintNoChannel_StaysPlainRefresh is the
// negative space of the widened gate: an arrow with neither a constraint
// nor a tracked channel has nothing for upgradeRef to resolve toward, so
// UpgradeRef must still fall through to plainRefresh exactly as before.
func TestArrowUpdate_UpgradeRef_NoConstraintNoChannel_StaysPlainRefresh(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1")
	current := &domain.Arrow{Namespace: ns}
	newArrow := &domain.Arrow{Namespace: ns}
	refreshCalled := false

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		RefreshManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			refreshCalled = true
			return newArrow, nil
		},
		UpdateManifestFn: func(_ context.Context, _ domain.Namespace, _ *domain.Arrow) error { return nil },
		ResolveTrackedRefFn: func(_ context.Context, _ domain.Arrow) (string, error) {
			t.Fatal("ResolveTrackedRef must not run when neither a constraint nor a channel is tracked")
			return "", nil
		},
	}
	g := &ucmocks.MockGraph{DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff { return graph.DepDiff{} }}

	uc := NewArrowUsecase(a, g, &ucmocks.MockRuntime{})
	if _, err := uc.Update(context.Background(), ns, models.UpdateOptions{UpgradeRef: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !refreshCalled {
		t.Fatal("expected plainRefresh's RefreshManifest to run")
	}
}

// TestArrowUpdate_UpgradeRef_PrefersRecommendedRef proves upgradeRef reuses
// an already-computed RecommendedRef outright rather than re-resolving —
// the exact value the passive drift-check already showed the caller as
// "the recommended upgrade", so the badge and the actual upgrade can never
// disagree, and ResolveTrackedRef (a live resolution) must not run at all.
func TestArrowUpdate_UpgradeRef_PrefersRecommendedRef(t *testing.T) {
	oldNs := domain.Namespace("test/arrow@v1.0.0")
	newNs := domain.Namespace("test/arrow@v1.2.0")
	current := &domain.Arrow{Namespace: oldNs, InstalledConstraint: "^v1", RecommendedRef: "v1.2.0"}
	var upgradeNewNs domain.Namespace

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ResolveTrackedRefFn: func(_ context.Context, _ domain.Arrow) (string, error) {
			t.Fatal("ResolveTrackedRef must not run when RecommendedRef is already set")
			return "", nil
		},
		UpgradeVersionFn: func(_ context.Context, _, newArg domain.Namespace, _, _ string, _, _, _ bool) (*domain.Arrow, error) {
			upgradeNewNs = newArg
			return &domain.Arrow{Namespace: newArg}, nil
		},
	}
	g := &ucmocks.MockGraph{DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff { return graph.DepDiff{} }}
	rt := &ucmocks.MockRuntime{
		RuntimeExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, nil },
	}

	uc := NewArrowUsecase(a, g, rt)
	if _, err := uc.Update(context.Background(), oldNs, models.UpdateOptions{UpgradeRef: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if upgradeNewNs != newNs {
		t.Fatalf("UpgradeVersion called with newNs=%q, want %q (from RecommendedRef)", upgradeNewNs, newNs)
	}
}

// TestArrowUpdate_UpgradeRef_CarriesChannelForwardAfterUpgrade guards a bug
// class this whole feature set exists to kill: without this, a channel-
// tracked arrow would lose its tracked channel on its very first upgrade.
// Channel now travels with the upgrade in the SAME event UpgradeVersion
// sends — passed as UpgradeVersion's own channel argument, not via any
// follow-up call — so this asserts the argument UpgradeVersion itself
// received, not a second command.
func TestArrowUpdate_UpgradeRef_CarriesChannelForwardAfterUpgrade(t *testing.T) {
	oldNs := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: oldNs, Channel: "beta", RecommendedRef: "v1.1.0-beta.1"}
	var gotChannel string
	var setChannelCalled bool

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		UpgradeVersionFn: func(_ context.Context, _, newArg domain.Namespace, _, channel string, _, _, _ bool) (*domain.Arrow, error) {
			gotChannel = channel
			return &domain.Arrow{Namespace: newArg}, nil
		},
		SetChannelFn: func(_ context.Context, _ domain.Namespace, _ string, _ string) error {
			setChannelCalled = true
			return nil
		},
	}
	g := &ucmocks.MockGraph{DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff { return graph.DepDiff{} }}
	rt := &ucmocks.MockRuntime{
		RuntimeExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, nil },
	}

	uc := NewArrowUsecase(a, g, rt)
	if _, err := uc.Update(context.Background(), oldNs, models.UpdateOptions{UpgradeRef: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotChannel != "beta" {
		t.Fatalf("UpgradeVersion called with channel=%q, want %q", gotChannel, "beta")
	}
	if setChannelCalled {
		t.Fatal("SetChannel must not be called by upgradeRef any more — the channel travels with UpgradeVersion's own event now")
	}
}

// TestArrowUpdate_UpgradeRef_NoChannel_PassesEmptyChannel is the guard's
// negative space: an arrow with no tracked channel passes an empty channel
// through to UpgradeVersion, not SetChannel avoidance (there is no separate
// SetChannel call to avoid any more).
func TestArrowUpdate_UpgradeRef_NoChannel_PassesEmptyChannel(t *testing.T) {
	oldNs := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: oldNs, InstalledConstraint: "^v1", RecommendedRef: "v1.1.0"}
	var gotChannel string

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		UpgradeVersionFn: func(_ context.Context, _, newArg domain.Namespace, _, channel string, _, _, _ bool) (*domain.Arrow, error) {
			gotChannel = channel
			return &domain.Arrow{Namespace: newArg}, nil
		},
	}
	g := &ucmocks.MockGraph{DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff { return graph.DepDiff{} }}
	rt := &ucmocks.MockRuntime{
		RuntimeExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, nil },
	}

	uc := NewArrowUsecase(a, g, rt)
	if _, err := uc.Update(context.Background(), oldNs, models.UpdateOptions{UpgradeRef: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotChannel != "" {
		t.Fatalf("UpgradeVersion called with channel=%q, want empty", gotChannel)
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

func TestArrowGetReadme_Success(t *testing.T) {
	ns := domain.Namespace("test/arrow")
	a := &ucmocks.MockArrow{
		ResolveManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns, Readme: "# Docs"}, nil
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	readme, err := uc.GetReadme(context.Background(), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if readme != "# Docs" {
		t.Fatalf("readme = %q, want %q", readme, "# Docs")
	}
}

func TestArrowGetReadme_EmptyReadme_NotFound(t *testing.T) {
	ns := domain.Namespace("test/arrow")
	a := &ucmocks.MockArrow{
		ResolveManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.GetReadme(context.Background(), ns); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestArrowGetReadme_ResolveManifestError(t *testing.T) {
	resolveErr := errors.New("resolve error")
	a := &ucmocks.MockArrow{
		ResolveManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return nil, resolveErr
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.GetReadme(context.Background(), "test/arrow"); !errors.Is(err, resolveErr) {
		t.Fatalf("expected resolveErr, got %v", err)
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

func TestArrowListChannels_DelegatesToArrow(t *testing.T) {
	target := domain.Namespace("test/arrow@v1")
	want := []models.ChannelInfo{
		{Name: "stable", Kind: "ordered", Latest: "v1.0.0", Count: 1, Members: []string{"v1.0.0"}},
	}
	called := false

	a := &ucmocks.MockArrow{
		ListChannelsFn: func(_ context.Context, ns domain.Namespace) ([]models.ChannelInfo, error) {
			called = true
			if ns != target {
				t.Errorf("got ns=%q, want %q", ns, target)
			}
			return want, nil
		},
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	got, err := uc.ListChannels(context.Background(), target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "stable" {
		t.Fatalf("got %v, want %v", got, want)
	}
	if !called {
		t.Fatal("expected arrow.ListChannels to be called")
	}
}

func TestArrowListChannels_PropagatesError(t *testing.T) {
	wantErr := errors.New("list tags unavailable")
	a := &ucmocks.MockArrow{
		ListChannelsFn: func(_ context.Context, _ domain.Namespace) ([]models.ChannelInfo, error) {
			return nil, wantErr
		},
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	_, err := uc.ListChannels(context.Background(), "test/arrow@v1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
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
		ResolveTrackedRefFn: func(_ context.Context, _ domain.Arrow) (string, error) { return "v1.0.0", nil },
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
		ResolveTrackedRefFn: func(_ context.Context, _ domain.Arrow) (string, error) { return "v1.0.0", nil },
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
		ResolveTrackedRefFn: func(_ context.Context, _ domain.Arrow) (string, error) { return "v1.1.0", nil },
		UpgradeVersionFn: func(_ context.Context, _, _ domain.Namespace, _, _ string, _, _, _ bool) (*domain.Arrow, error) {
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

// TestArrowUpdate_PlainRefresh_RunningRejected pins the guard Update has
// always had for a plain manifest refresh (no upgrade_ref): a running arrow
// is refused outright, unlike the upgrade_ref path below, which stops it
// first instead.
func TestArrowUpdate_PlainRefresh_RunningRejected(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns}
	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateRunning, nil
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, rt)
	_, err := uc.Update(context.Background(), ns, models.UpdateOptions{})
	if !errors.Is(err, apperrors.ErrStateViolation) {
		t.Fatalf("expected ErrStateViolation, got %v", err)
	}
}

// TestArrowUpdate_UpgradeRef_Running_StopsFirstThenUpgrades is the generic
// fix: any running arrow being moved to a new ref is stopped first, so
// UpgradeVersion's own reaction (onArrowUpgraded) finds it Ready and
// auto-continues instead of orphaning it.
func TestArrowUpdate_UpgradeRef_Running_StopsFirstThenUpgrades(t *testing.T) {
	oldNs := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: oldNs, InstalledConstraint: "^v1"}
	stopCalled := false
	upgradeCalled := false

	a := &ucmocks.MockArrow{
		GetFn:               func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ResolveTrackedRefFn: func(_ context.Context, _ domain.Arrow) (string, error) { return "v1.1.0", nil },
		UpgradeVersionFn: func(_ context.Context, _, newArg domain.Namespace, _, _ string, _, _, _ bool) (*domain.Arrow, error) {
			upgradeCalled = true
			return &domain.Arrow{Namespace: newArg}, nil
		},
	}
	ch := make(chan domainRuntime.ArrowRuntime, 1)
	ch <- domainRuntime.ArrowRuntime{Ref: oldNs, State: domain.ArrowStateReady}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateRunning, nil
		},
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			return ch, func() {}, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			stopCalled = true
			return nil
		},
		RuntimeExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, nil },
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, rt)
	_, err := uc.Update(context.Background(), oldNs, models.UpdateOptions{UpgradeRef: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stopCalled {
		t.Fatal("expected BeginStop to be called before the upgrade")
	}
	if !upgradeCalled {
		t.Fatal("expected UpgradeVersion to be called after the stop completed")
	}
}

func TestArrowUpdate_UpgradeRef_NotRunning_NoStopCalled(t *testing.T) {
	oldNs := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: oldNs, InstalledConstraint: "^v1"}
	stopCalled := false

	a := &ucmocks.MockArrow{
		GetFn:               func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ResolveTrackedRefFn: func(_ context.Context, _ domain.Arrow) (string, error) { return "v1.1.0", nil },
		UpgradeVersionFn: func(_ context.Context, _, newArg domain.Namespace, _, _ string, _, _, _ bool) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: newArg}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			stopCalled = true
			return nil
		},
		RuntimeExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) { return false, nil },
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, rt)
	if _, err := uc.Update(context.Background(), oldNs, models.UpdateOptions{UpgradeRef: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stopCalled {
		t.Fatal("expected no BeginStop call when the arrow was not running")
	}
}

func TestArrowUpdate_UpgradeRef_StopBeginStopError_ReturnsError(t *testing.T) {
	oldNs := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: oldNs, InstalledConstraint: "^v1"}
	stopErr := errors.New("stop failed")

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateRunning, nil
		},
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			return make(chan domainRuntime.ArrowRuntime), func() {}, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			return stopErr
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, rt)
	_, err := uc.Update(context.Background(), oldNs, models.UpdateOptions{UpgradeRef: true})
	if !errors.Is(err, stopErr) {
		t.Fatalf("expected stopErr, got %v", err)
	}
}

func TestArrowUpdate_UpgradeRef_StopGetStateError_ReturnsError(t *testing.T) {
	oldNs := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: oldNs, InstalledConstraint: "^v1"}
	stateErr := errors.New("state unavailable")

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return "", stateErr
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, rt)
	_, err := uc.Update(context.Background(), oldNs, models.UpdateOptions{UpgradeRef: true})
	if !errors.Is(err, stateErr) {
		t.Fatalf("expected stateErr, got %v", err)
	}
}

func TestArrowUpdate_UpgradeRef_StopListenEndedError_ReturnsError(t *testing.T) {
	oldNs := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: oldNs, InstalledConstraint: "^v1"}
	listenErr := errors.New("listen unavailable")

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateRunning, nil
		},
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			return nil, func() {}, listenErr
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, rt)
	_, err := uc.Update(context.Background(), oldNs, models.UpdateOptions{UpgradeRef: true})
	if !errors.Is(err, listenErr) {
		t.Fatalf("expected listenErr, got %v", err)
	}
}

func TestArrowUpdate_UpgradeRef_StopContextCancelled_ReturnsError(t *testing.T) {
	oldNs := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: oldNs, InstalledConstraint: "^v1"}

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateRunning, nil
		},
		ListenEndedFn: func(_ context.Context, _ domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			return make(chan domainRuntime.ArrowRuntime), func() {}, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			return nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, rt)
	_, err := uc.Update(ctx, oldNs, models.UpdateOptions{UpgradeRef: true})
	if err == nil {
		t.Fatal("expected an error when the context is cancelled while waiting for the stop")
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

func TestArrowUsecase_Remove_BareNamespaceResolvesToCataloguedRef(t *testing.T) {
	var removed domain.Namespace

	arrow := &ucmocks.MockArrow{
		ResolveCataloguedFn: func(_ context.Context, ns domain.Namespace) (domain.Namespace, error) {
			if ns != domain.Namespace("github.com/u/r") {
				t.Errorf("expected github.com/u/r, got %q", ns)
			}
			return domain.Namespace("github.com/u/r@main"), nil
		},
		RemoveFn: func(_ context.Context, ns domain.Namespace) error {
			removed = ns
			return nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
	}
	gr := &ucmocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _, _ domain.Namespace) (bool, error) {
			return false, nil
		},
	}

	uc := NewArrowUsecase(arrow, gr, rt)

	if err := uc.Remove(context.Background(), "github.com/u/r"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed != domain.Namespace("github.com/u/r@main") {
		t.Errorf("expected removed to be github.com/u/r@main, got %q", removed)
	}
}

func TestArrowUsecase_Update_BareNamespaceResolvesToCataloguedRef(t *testing.T) {
	var got domain.Namespace

	arrow := &ucmocks.MockArrow{
		ResolveCataloguedFn: func(_ context.Context, ns domain.Namespace) (domain.Namespace, error) {
			if ns != domain.Namespace("github.com/u/r") {
				t.Errorf("expected github.com/u/r, got %q", ns)
			}
			return domain.Namespace("github.com/u/r@main"), nil
		},
		GetFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			got = ns
			return &domain.Arrow{Namespace: ns}, nil
		},
		RefreshManifestFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		UpdateManifestFn: func(_ context.Context, _ domain.Namespace, _ *domain.Arrow) error {
			return nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
	}
	gr := &ucmocks.MockGraph{
		DiffDepsFn: func(_, _ *domain.Arrow) graph.DepDiff { return graph.DepDiff{} },
	}

	uc := NewArrowUsecase(arrow, gr, rt)

	_, err := uc.Update(context.Background(), "github.com/u/r", models.UpdateOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != domain.Namespace("github.com/u/r@main") {
		t.Errorf("expected got to be github.com/u/r@main, got %q", got)
	}
}

func TestArrowGetManifest_ExplicitRef_Resolves(t *testing.T) {
	ns := domain.Namespace("github.com/char2cs/crowbar@v1.2.0")
	a := &ucmocks.MockArrow{
		ResolveManifestFn: func(_ context.Context, resolveNs domain.Namespace) (*domain.Arrow, error) {
			if resolveNs != ns {
				t.Errorf("got ns=%q, want %q", resolveNs, ns)
			}
			return &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Crowbar"}}, nil
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})

	got, err := uc.GetManifest(context.Background(), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Crowbar" {
		t.Fatalf("got name=%q, want %q", got.Name, "Crowbar")
	}
}

func TestArrowGetReadme_ExplicitRef_Resolves(t *testing.T) {
	ns := domain.Namespace("github.com/char2cs/crowbar@v1.2.0")
	a := &ucmocks.MockArrow{
		ResolveManifestFn: func(_ context.Context, resolveNs domain.Namespace) (*domain.Arrow, error) {
			if resolveNs != ns {
				t.Errorf("got ns=%q, want %q", resolveNs, ns)
			}
			return &domain.Arrow{Namespace: ns, Readme: "hello"}, nil
		},
	}
	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})

	got, err := uc.GetReadme(context.Background(), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("got readme=%q, want %q", got, "hello")
	}
}

// ─── Update: switchChannel ────────────────────────────────────────────────

// TestArrowUpdate_SwitchChannel_RecordsPreferenceAndTriggersCheck is the
// core of the new, simplified switchChannel: it records the channel via
// SetChannel and kicks off an immediate, asynchronous version check — it
// never performs a live upgrade inline, and the result is always the
// zero-value UpdateResult (there is nothing else to report synchronously).
//
// current carries an InstalledConstraint only to mirror a realistic
// glob-installed arrow's shape — this test does not, and cannot, prove
// that the constraint gets cleared: SetChannel is mocked here, so its
// real EmitEvent never runs. That guarantee is proven at the repository/
// command layer instead: see
// TestSetChannel_OnConstraintTrackedArrow_ClearsConstraintSoDriftUsesChannel
// and TestSetChannel_EmitEvent_ClearsInstalledConstraint.
func TestArrowUpdate_SwitchChannel_RecordsPreferenceAndTriggersCheck(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns, InstalledConstraint: "^v1"}
	channels := []models.ChannelInfo{
		{Name: "beta", Kind: "ordered", Latest: "v1.2.0-beta.1", Count: 2, Members: []string{"v1.2.0-beta.1", "v1.1.0-beta.1"}},
	}
	setChannelCalled := ""
	var setChannelRef string
	var checkNowNs domain.Namespace

	a := &ucmocks.MockArrow{
		GetFn:          func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ListChannelsFn: func(_ context.Context, _ domain.Namespace) ([]models.ChannelInfo, error) { return channels, nil },
		SetChannelFn: func(_ context.Context, _ domain.Namespace, channel string, ref string) error {
			setChannelCalled = channel
			setChannelRef = ref
			return nil
		},
		CheckVersionNowFn: func(_ context.Context, checkNs domain.Namespace) {
			checkNowNs = checkNs
		},
		UpgradeVersionFn: func(_ context.Context, _, _ domain.Namespace, _, _ string, _, _, _ bool) (*domain.Arrow, error) {
			t.Fatal("UpgradeVersion must never be called from switchChannel any more")
			return nil, nil
		},
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	result, err := uc.Update(context.Background(), ns, models.UpdateOptions{Channel: "beta"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if setChannelCalled != "beta" {
		t.Fatalf("SetChannel called with %q, want %q", setChannelCalled, "beta")
	}
	if setChannelRef != "" {
		t.Fatalf("SetChannel called with ref %q, want empty (no pin requested)", setChannelRef)
	}
	if checkNowNs != ns {
		t.Fatalf("CheckVersionNow called with %q, want %q", checkNowNs, ns)
	}
	zero := len(result.AddedDeps) == 0 && len(result.RemovedFromManifest) == 0 &&
		len(result.SafeToUninstall) == 0 && len(result.ConstrainedDeps) == 0 && result.NewRef == ""
	if !zero {
		t.Fatalf("expected a zero-value UpdateResult, got %+v", result)
	}
}

// TestArrowUpdate_SwitchChannel_ExplicitRef_ValidatedAndPinned proves a
// pinned ref within the channel is both validated (channelHasRef, unchanged
// from before) and carried forward to SetChannel as the pin to track within
// that channel — it still never drives any upgrade inline, that stays
// exclusively upgradeRef's job.
func TestArrowUpdate_SwitchChannel_ExplicitRef_ValidatedAndPinned(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns, InstalledConstraint: "^v1"}
	channels := []models.ChannelInfo{
		{Name: "beta", Kind: "ordered", Latest: "v1.2.0-beta.1", Count: 2, Members: []string{"v1.2.0-beta.1", "v1.1.0-beta.1"}},
	}
	var setChannelNs domain.Namespace
	var setChannelRef string

	a := &ucmocks.MockArrow{
		GetFn:          func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ListChannelsFn: func(_ context.Context, _ domain.Namespace) ([]models.ChannelInfo, error) { return channels, nil },
		SetChannelFn: func(_ context.Context, target domain.Namespace, _ string, ref string) error {
			setChannelNs = target
			setChannelRef = ref
			return nil
		},
		UpgradeVersionFn: func(_ context.Context, _, _ domain.Namespace, _, _ string, _, _, _ bool) (*domain.Arrow, error) {
			t.Fatal("UpgradeVersion must not be called even when a valid pinned ref is given")
			return nil, nil
		},
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.Update(context.Background(), ns, models.UpdateOptions{Channel: "beta", Ref: "v1.1.0-beta.1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if setChannelNs != ns {
		t.Fatalf("SetChannel called with %q, want the original ns %q (no ref swap)", setChannelNs, ns)
	}
	if setChannelRef != "v1.1.0-beta.1" {
		t.Fatalf("SetChannel called with ref %q, want the pinned ref %q", setChannelRef, "v1.1.0-beta.1")
	}
}

// TestArrowUpdate_SwitchChannel_PointerChannel_ArbitraryRefAccepted proves a
// pointer channel's validation still accepts any non-empty ref, not just its
// own Latest — a rolling channel (a branch, or an unversioned tag) has no
// fixed member list by definition. See channelHasRef's own doc comment.
func TestArrowUpdate_SwitchChannel_PointerChannel_ArbitraryRefAccepted(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns}
	channels := []models.ChannelInfo{
		{Name: "main", Kind: "pointer", Latest: "main"},
	}

	a := &ucmocks.MockArrow{
		GetFn:          func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ListChannelsFn: func(_ context.Context, _ domain.Namespace) ([]models.ChannelInfo, error) { return channels, nil },
		SetChannelFn:   func(_ context.Context, _ domain.Namespace, _ string, _ string) error { return nil },
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.Update(context.Background(), ns, models.UpdateOptions{Channel: "main", Ref: "dev"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestArrowUpdate_SwitchChannel_NeverStopsOrUpgrades_EvenWhileRunning is the
// direct assertion the redesign exists for: switchChannel must never touch
// runtime state or perform an upgrade at all, regardless of whether the
// arrow happens to be running — that safety concern belongs exclusively to
// upgradeRef now.
func TestArrowUpdate_SwitchChannel_NeverStopsOrUpgrades_EvenWhileRunning(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns}
	channels := []models.ChannelInfo{
		{Name: "beta", Kind: "ordered", Latest: "v1.1.0-beta.1", Count: 1, Members: []string{"v1.1.0-beta.1"}},
	}
	setChannelCalled := false

	a := &ucmocks.MockArrow{
		GetFn:          func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ListChannelsFn: func(_ context.Context, _ domain.Namespace) ([]models.ChannelInfo, error) { return channels, nil },
		SetChannelFn: func(_ context.Context, _ domain.Namespace, _ string, _ string) error {
			setChannelCalled = true
			return nil
		},
		UpgradeVersionFn: func(_ context.Context, _, _ domain.Namespace, _, _ string, _, _, _ bool) (*domain.Arrow, error) {
			t.Fatal("UpgradeVersion must not be called by switchChannel")
			return nil, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			t.Fatal("switchChannel must never check runtime state — that was upgradeRef/stopIfRunning's job")
			return domain.ArrowStateRunning, nil
		},
		BeginStopFn: func(_ context.Context, _ domain.Namespace) error {
			t.Fatal("switchChannel must never call BeginStop")
			return nil
		},
		RuntimeExistsFn: func(_ context.Context, _ domain.Namespace) (bool, error) {
			t.Fatal("switchChannel must never call RuntimeExists")
			return false, nil
		},
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, rt)
	if _, err := uc.Update(context.Background(), ns, models.UpdateOptions{Channel: "beta"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !setChannelCalled {
		t.Fatal("expected SetChannel to still be called")
	}
}

// TestArrowUpdate_SwitchChannel_SetChannelError_ReturnsError also confirms
// CheckVersionNow is never reached when SetChannel itself fails.
func TestArrowUpdate_SwitchChannel_SetChannelError_ReturnsError(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns}
	wantErr := errors.New("set channel failed")
	channels := []models.ChannelInfo{
		{Name: "stable", Kind: "ordered", Latest: "v1.0.0", Count: 1, Members: []string{"v1.0.0"}},
	}

	a := &ucmocks.MockArrow{
		GetFn:          func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ListChannelsFn: func(_ context.Context, _ domain.Namespace) ([]models.ChannelInfo, error) { return channels, nil },
		SetChannelFn:   func(_ context.Context, _ domain.Namespace, _ string, _ string) error { return wantErr },
		CheckVersionNowFn: func(_ context.Context, _ domain.Namespace) {
			t.Fatal("CheckVersionNow must not run when SetChannel fails")
		},
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	_, err := uc.Update(context.Background(), ns, models.UpdateOptions{Channel: "stable"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wantErr, got %v", err)
	}
}

func TestArrowUpdate_SwitchChannel_ListChannelsError(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns}
	wantErr := errors.New("list channels unavailable")

	a := &ucmocks.MockArrow{
		GetFn:          func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ListChannelsFn: func(_ context.Context, _ domain.Namespace) ([]models.ChannelInfo, error) { return nil, wantErr },
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	_, err := uc.Update(context.Background(), ns, models.UpdateOptions{Channel: "beta"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wantErr, got %v", err)
	}
	if errors.Is(err, apperrors.ErrChannelNotFound) {
		t.Fatal("a ListChannels failure is not a channel-not-found condition")
	}
}

func TestArrowUpdate_SwitchChannel_ChannelNotFound(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns}
	channels := []models.ChannelInfo{
		{Name: "stable", Kind: "ordered", Latest: "v1.0.0", Count: 1, Members: []string{"v1.0.0"}},
	}

	a := &ucmocks.MockArrow{
		GetFn:          func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ListChannelsFn: func(_ context.Context, _ domain.Namespace) ([]models.ChannelInfo, error) { return channels, nil },
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	_, err := uc.Update(context.Background(), ns, models.UpdateOptions{Channel: "nightly"})
	if !errors.Is(err, apperrors.ErrChannelNotFound) {
		t.Fatalf("expected ErrChannelNotFound, got %v", err)
	}
}

func TestArrowUpdate_SwitchChannel_RefNotInChannel(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns}
	channels := []models.ChannelInfo{
		{Name: "beta", Kind: "ordered", Latest: "v1.2.0-beta.1", Count: 2, Members: []string{"v1.2.0-beta.1", "v1.1.0-beta.1"}},
	}

	a := &ucmocks.MockArrow{
		GetFn:          func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ListChannelsFn: func(_ context.Context, _ domain.Namespace) ([]models.ChannelInfo, error) { return channels, nil },
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	_, err := uc.Update(context.Background(), ns, models.UpdateOptions{Channel: "beta", Ref: "v1.0.0-beta.1"})
	if !errors.Is(err, apperrors.ErrChannelNotFound) {
		t.Fatalf("expected ErrChannelNotFound, got %v", err)
	}
}

// TestArrowUpdate_SwitchChannel_TakesPrecedenceOverUpgradeRef pins that a
// non-empty Channel is handled before UpgradeRef is even consulted, even
// when both are set on the same request.
func TestArrowUpdate_SwitchChannel_TakesPrecedenceOverUpgradeRef(t *testing.T) {
	ns := domain.Namespace("test/arrow@v1.0.0")
	current := &domain.Arrow{Namespace: ns, InstalledConstraint: "^v1"}
	channels := []models.ChannelInfo{
		{Name: "stable", Kind: "ordered", Latest: "v1.0.0", Count: 1, Members: []string{"v1.0.0"}},
	}
	listChannelsCalled := false

	a := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return current, nil },
		ListChannelsFn: func(_ context.Context, _ domain.Namespace) ([]models.ChannelInfo, error) {
			listChannelsCalled = true
			return channels, nil
		},
		SetChannelFn: func(_ context.Context, _ domain.Namespace, _ string, _ string) error { return nil },
		ResolveConstraintFn: func(_ context.Context, _ domain.Namespace, _ string) (string, error) {
			t.Fatal("ResolveConstraint (upgradeRef's path) must not run when Channel is also set")
			return "", nil
		},
		ResolveTrackedRefFn: func(_ context.Context, _ domain.Arrow) (string, error) {
			t.Fatal("ResolveTrackedRef (upgradeRef's path) must not run when Channel is also set")
			return "", nil
		},
	}

	uc := NewArrowUsecase(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.Update(context.Background(), ns, models.UpdateOptions{Channel: "stable", UpgradeRef: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !listChannelsCalled {
		t.Fatal("expected the channel-switch path (ListChannels) to run")
	}
}
