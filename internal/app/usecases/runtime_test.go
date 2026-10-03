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

// newUC wires the usecase to a real lifecycle over the same mocks, so a test
// observes what reaches the repositories.
func newUC(a *ucmocks.MockArrow, rt *ucmocks.MockRuntime, g *ucmocks.MockGraph) RuntimeUsecase {
	return NewRuntimeUsecase(a, rt, lifecycle.New(a, rt, g))
}

func TestRuntimeNewUsecase_DoesNotPanic(t *testing.T) {
	uc := newUC(&ucmocks.MockArrow{}, &ucmocks.MockRuntime{}, &ucmocks.MockGraph{})
	if uc == nil {
		t.Fatal("expected non-nil usecase")
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

// Every built-in is a fact about the execution, so a request that sets one is
// asking for something Quiver cannot honour and must be told so.
func TestRuntimeUsecase_ReservedVariable_RejectedOnEveryEntryPoint(t *testing.T) {
	entryPoints := map[string]func(uc RuntimeUsecase, vars map[string]string) error{
		"install": func(uc RuntimeUsecase, vars map[string]string) error {
			_, err := uc.Install(context.Background(), "github.com/user/repo@v1", vars)
			return err
		},
		"uninstall": func(uc RuntimeUsecase, vars map[string]string) error {
			return uc.Uninstall(context.Background(), "github.com/user/repo@v1", vars)
		},
		"execute": func(uc RuntimeUsecase, vars map[string]string) error {
			return uc.Execute(context.Background(), "github.com/user/repo@v1", domain.MethodExecute, vars)
		},
		"update": func(uc RuntimeUsecase, vars map[string]string) error {
			_, err := uc.Update(context.Background(), "github.com/user/repo@v1", vars)
			return err
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
					BeginUpdateFn: func(context.Context, domain.Namespace, map[string]string, string) error {
						reached = true
						return nil
					},
				}
				uc := newUC(&ucmocks.MockArrow{}, rt, &ucmocks.MockGraph{})

				err := call(uc, map[string]string{name: "hijacked"})

				if !errors.Is(err, apperrors.ErrReservedVariable) {
					t.Fatalf("expected ErrReservedVariable, got %v", err)
				}
				if reached {
					t.Fatal("request reached the runtime repository instead of being rejected")
				}
			})
		}
	}
}

func TestRuntimeUsecase_SeveralReservedVariables_RejectsDeterministically(t *testing.T) {
	uc := newUC(&ucmocks.MockArrow{}, &ucmocks.MockRuntime{}, &ucmocks.MockGraph{})

	vars := map[string]string{}
	for _, name := range domain.ReservedVariableNames() {
		vars[name] = "hijacked"
	}

	_, first := uc.Install(context.Background(), "github.com/user/repo@v1", vars)
	require.ErrorIs(t, first, apperrors.ErrReservedVariable)
	for range 20 {
		_, err := uc.Install(context.Background(), "github.com/user/repo@v1", vars)
		assert.Equal(t, first, err)
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
		GetFn:    func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) { return &domain.Arrow{}, nil },
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

func TestRuntimeGetRuntime_ResolvesBareNamespace(t *testing.T) {
	bare := domain.Namespace("github.com/user/app")
	versioned := domain.Namespace("github.com/user/app@main")

	a := &ucmocks.MockArrow{
		ResolveCataloguedFn: func(_ context.Context, ns domain.Namespace) (domain.Namespace, error) {
			if ns == bare {
				return versioned, nil
			}
			return ns, nil
		},
		ExistsFn: func(_ context.Context, ns domain.Namespace) (bool, error) {
			return ns == versioned, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			if ns != versioned {
				return nil, nil
			}
			return &domainRuntime.ArrowRuntime{Ref: versioned, State: domain.ArrowStateReady}, nil
		},
	}
	uc := newUC(a, rt, &ucmocks.MockGraph{})

	got, err := uc.GetRuntime(context.Background(), bare)

	require.NoError(t, err)
	assert.Equal(t, versioned, got.Ref)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

func TestRuntimeUsecase_ThinVerbs_ReachTheLifecycle(t *testing.T) {
	ns := domain.Namespace("github.com/user/app@stable")
	var calls []string
	lc := &ucmocks.MockLifecycle{
		UpdateFn: func(context.Context, domain.Namespace, map[string]string) (bool, error) {
			calls = append(calls, "update")
			return true, nil
		},
		StopFn: func(context.Context, domain.Namespace) error {
			calls = append(calls, "stop")
			return nil
		},
		ResetFn: func(context.Context, domain.Namespace) error {
			calls = append(calls, "reset")
			return nil
		},
		SettlingFn: func(domain.Namespace) bool {
			calls = append(calls, "settling")
			return true
		},
		DrainFn: func(context.Context) error {
			calls = append(calls, "drain")
			return assert.AnError
		},
	}
	uc := NewRuntimeUsecase(&ucmocks.MockArrow{}, &ucmocks.MockRuntime{}, lc)
	ctx := context.Background()

	started, err := uc.Update(ctx, ns, nil)
	require.NoError(t, err)
	assert.True(t, started)
	require.NoError(t, uc.Stop(ctx, ns))
	require.NoError(t, uc.Reset(ctx, ns))
	assert.True(t, uc.Settling(ns))
	require.ErrorIs(t, uc.Drain(ctx), assert.AnError)

	assert.Equal(t, []string{"update", "stop", "reset", "settling", "drain"}, calls)
}

func TestRuntimeGetRuntime_Failures(t *testing.T) {
	ns := domain.Namespace("github.com/user/app@stable")

	testCases := []struct {
		name       string
		resolveErr error
		runtimeErr error
		exists     bool
		existsErr  error
		wantErr    error
		wantState  domain.ArrowState
	}{
		{name: "not catalogued", resolveErr: apperrors.ErrNotFound, wantErr: apperrors.ErrNotFound},
		{name: "runtime unreadable", runtimeErr: assert.AnError, wantErr: assert.AnError},
		{name: "catalog unreadable", existsErr: assert.AnError, wantErr: assert.AnError},
		{name: "no such arrow", wantErr: apperrors.ErrNotFound},
		{name: "catalogued but never installed", exists: true, wantState: domain.ArrowStateAbsent},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := &ucmocks.MockArrow{
				ResolveCataloguedFn: func(_ context.Context, got domain.Namespace) (domain.Namespace, error) {
					return got, tc.resolveErr
				},
				ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return tc.exists, tc.existsErr },
			}
			rt := &ucmocks.MockRuntime{
				GetRuntimeFn: func(context.Context, domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
					return nil, tc.runtimeErr
				},
			}

			got, err := newUC(a, rt, &ucmocks.MockGraph{}).GetRuntime(context.Background(), ns)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantState, got.State)
		})
	}
}

func TestRuntimeUsecase_Activate_ReachesTheLifecycle(t *testing.T) {
	ns := domain.Namespace("github.com/user/app@stable")
	testCases := []struct {
		name    string
		started bool
		err     error
	}{
		{name: "started", started: true},
		{name: "nothing staged", started: false},
		{name: "refused", err: assert.AnError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			lc := &ucmocks.MockLifecycle{
				ActivateFn: func(_ context.Context, got domain.Namespace) (bool, error) {
					assert.Equal(t, ns, got)
					return tc.started, tc.err
				},
			}
			uc := NewRuntimeUsecase(&ucmocks.MockArrow{}, &ucmocks.MockRuntime{}, lc)

			started, err := uc.Activate(context.Background(), ns)

			assert.Equal(t, tc.started, started)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
		})
	}
}
