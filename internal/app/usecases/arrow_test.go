package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// newArrowUC wires the usecase to a real lifecycle over the same mocks.
func newArrowUC(a *ucmocks.MockArrow, g *ucmocks.MockGraph, rt *ucmocks.MockRuntime) ArrowUsecase {
	return NewArrowUsecase(a, g, rt, lifecycle.New(a, rt, g))
}

// --- tests ---

func TestArrowRemove_StateViolationGuard(t *testing.T) {
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateRunning, nil
		},
	}

	uc := newArrowUC(&ucmocks.MockArrow{}, &ucmocks.MockGraph{}, rt)
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

	uc := newArrowUC(a, &ucmocks.MockGraph{}, rt)
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

	uc := newArrowUC(&ucmocks.MockArrow{}, g, &ucmocks.MockRuntime{})
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

	uc := newArrowUC(&ucmocks.MockArrow{}, g, &ucmocks.MockRuntime{})
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

	uc := newArrowUC(&ucmocks.MockArrow{}, g, &ucmocks.MockRuntime{})
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

	uc := newArrowUC(&ucmocks.MockArrow{}, g, &ucmocks.MockRuntime{})
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

	uc := newArrowUC(&ucmocks.MockArrow{}, g, &ucmocks.MockRuntime{})
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

	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
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

	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
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
	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
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
	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
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
	uc := newArrowUC(&ucmocks.MockArrow{}, &ucmocks.MockGraph{}, rt)
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
	uc := newArrowUC(&ucmocks.MockArrow{}, g, rt)
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
	uc := newArrowUC(&ucmocks.MockArrow{}, g, rt)
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
	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.List(context.Background(), nil); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

// TestArrowUpdate_ResolveCataloguedError_ReturnsError covers Update's first
// step, distinct from TestArrowUpdate_PropagatesGetError which covers the
// second (u.arrow.Get) — both must independently propagate.
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

	uc := newArrowUC(a, &ucmocks.MockGraph{}, mockRT)
	dto, err := uc.GetDetail(context.Background(), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dto == nil {
		t.Fatal("expected non-nil DTO")
	}
}

// TestArrowGetDetail_BareNamespace_ResolvesRuntimeAgainstTrackedRef guards a
// real bug: GetDetail used to look up the runtime with the caller's own ns
// verbatim, even when it was a bare namespace (no @ref) that u.arrow.GetDetail
// had already resolved to the tracked ref on view.Metadata.Namespace. The
// runtime repository's aggregates are keyed by the exact ref-qualified
// namespace string, so looking it up under the still-bare ns silently missed
// every genuinely installed arrow queried without an explicit ref -- exactly
// how the CLI and a bare `GET /v0/arrow/:ns` call both reach this path.
func TestArrowGetDetail_BareNamespace_ResolvesRuntimeAgainstTrackedRef(t *testing.T) {
	bareNs := domain.Namespace("test/arrow")
	resolvedNs := domain.Namespace("test/arrow@v1")
	detail := &models.ArrowDetailView{Metadata: domain.Arrow{Namespace: resolvedNs}}
	rt := &domainRuntime.ArrowRuntime{Ref: resolvedNs, State: domain.ArrowStateReady}
	var gotNs domain.Namespace

	a := &ucmocks.MockArrow{
		GetDetailFn: func(_ context.Context, _ domain.Namespace) (*models.ArrowDetailView, error) {
			return detail, nil
		},
	}
	mockRT := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			gotNs = ns
			return rt, nil
		},
	}

	uc := newArrowUC(a, &ucmocks.MockGraph{}, mockRT)
	dto, err := uc.GetDetail(context.Background(), bareNs)
	require.NoError(t, err)
	require.NotNil(t, dto)
	assert.Equal(t, resolvedNs, gotNs, "runtime must be looked up against the resolved, ref-qualified namespace, not the caller's bare one")
	assert.Equal(t, domain.ArrowStateReady, dto.State)
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

	uc := newArrowUC(a, &ucmocks.MockGraph{}, mockRT)
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
	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
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
	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
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
	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
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
	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
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
	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	if _, err := uc.GetReadme(context.Background(), "test/arrow"); !errors.Is(err, resolveErr) {
		t.Fatalf("expected resolveErr, got %v", err)
	}
}

func TestArrowSeed_AdoptsAPinOfItsOwnRef(t *testing.T) {
	var (
		gotNs       domain.Namespace
		gotKind     domain.SelectorKind
		gotResolved domain.Resolved
		gotManifest []byte
		gotFilename string
	)
	a := &ucmocks.MockArrow{
		AdoptFn: func(
			_ context.Context,
			ns domain.Namespace,
			kind domain.SelectorKind,
			resolved domain.Resolved,
			manifest []byte,
			filename string,
		) error {
			gotNs, gotKind, gotResolved, gotManifest, gotFilename = ns, kind, resolved, manifest, filename
			return nil
		},
	}
	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})

	require.NoError(t, uc.Seed(context.Background(), "test/arrow/x@v1", []byte("data")))

	assert.Equal(t, domain.Namespace("test/arrow/x@v1"), gotNs)
	assert.Equal(t, domain.SelectorPin, gotKind)
	assert.Equal(t, domain.Resolved{Ref: "v1"}, gotResolved)
	assert.Equal(t, []byte("data"), gotManifest)
	assert.Equal(t, "ARROW.md", gotFilename)
}

func TestArrowValidateManifest_DelegatesToArrow(t *testing.T) {
	result := &models.ValidationResult{}
	a := &ucmocks.MockArrow{
		ValidateManifestFn: func(_ context.Context, _ []byte) (*models.ValidationResult, error) {
			return result, nil
		},
	}
	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
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

	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
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

	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
	_, err := uc.ListChannels(context.Background(), "test/arrow@v1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
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
	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})
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

	uc := newArrowUC(arrow, gr, rt)

	if err := uc.Remove(context.Background(), "github.com/u/r"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed != domain.Namespace("github.com/u/r@main") {
		t.Errorf("expected removed to be github.com/u/r@main, got %q", removed)
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
	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})

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
	uc := newArrowUC(a, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{})

	got, err := uc.GetReadme(context.Background(), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("got readme=%q, want %q", got, "hello")
	}
}

func TestArrowAdoptInstalled_DelegatesAndLeavesTheRuntimeAlone(t *testing.T) {
	adoptErr := errors.New("adopt error")
	testCases := []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "adopted"},
		{name: "refused", err: adoptErr, wantErr: adoptErr},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var gotNs domain.Namespace
			var gotRef string
			a := &ucmocks.MockArrow{
				AdoptInstalledFn: func(_ context.Context, ns domain.Namespace, ref string) error {
					gotNs, gotRef = ns, ref
					return tc.err
				},
			}
			rt := &ucmocks.MockRuntime{
				GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
					t.Error("the runtime must not be read")
					return "", nil
				},
			}
			uc := newArrowUC(a, &ucmocks.MockGraph{}, rt)

			err := uc.AdoptInstalled(context.Background(), "github.com/u/r@stable", "v1.2.0")

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			if gotNs != "github.com/u/r@stable" || gotRef != "v1.2.0" {
				t.Fatalf("expected the identity and ref to reach the catalog, got %q %q", gotNs, gotRef)
			}
		})
	}
}

func TestArrowUsecase_Update_RechecksThroughTheLifecycle(t *testing.T) {
	ns := domain.Namespace("github.com/user/app@stable")
	want := models.UpdateResult{Available: &domain.Available{Ref: "stable", Commit: "c2"}}
	lc := &ucmocks.MockLifecycle{
		RecheckFn: func(_ context.Context, got domain.Namespace) (models.UpdateResult, error) {
			assert.Equal(t, ns, got)
			return want, nil
		},
	}

	got, err := NewArrowUsecase(&ucmocks.MockArrow{}, &ucmocks.MockGraph{}, &ucmocks.MockRuntime{}, lc).Update(context.Background(), ns)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}
