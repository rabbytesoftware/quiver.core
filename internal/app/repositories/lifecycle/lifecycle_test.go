package lifecycle_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

const row = domain.Namespace("github.com/user/app@stable")

func TestContainerNew_OnRuntimeEndedError(t *testing.T) {
	expected := errors.New("subscribe error")
	rt := &mocks.MockRuntime{
		OnRuntimeEndedFn: func(func(context.Context, domainRuntime.ArrowRuntime)) error {
			return expected
		},
	}

	err := lifecycle.New(&mocks.MockArrow{}, rt, &mocks.MockGraph{}).Start()

	require.ErrorIs(t, err, expected)
}

func TestContainerNew_WiresOnRuntimeEnded(t *testing.T) {
	var reaction func(context.Context, domainRuntime.ArrowRuntime)
	stopped := false
	rt := &mocks.MockRuntime{
		OnRuntimeEndedFn: func(fn func(context.Context, domainRuntime.ArrowRuntime)) error {
			reaction = fn
			return nil
		},
		GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateRunning, nil
		},
		BeginStopFn: func(context.Context, domain.Namespace) error {
			stopped = true
			return nil
		},
	}
	g := &mocks.MockGraph{
		ResolveFn: func(context.Context, domain.Namespace) (models.Plan, error) {
			return models.Plan{{Namespace: "github.com/user/svc@v1", Type: domain.ServiceDep}}, nil
		},
	}

	require.NoError(t, lifecycle.New(&mocks.MockArrow{}, rt, g).Start())
	require.NotNil(t, reaction, "the reaction to a runtime's end is subscribed")

	reaction(context.Background(), domainRuntime.ArrowRuntime{
		Ref:        row,
		LastReturn: &domainRuntime.Return{Method: domain.MethodStop},
	})

	assert.True(t, stopped, "a stop cascades to the service it no longer needs")
}

func TestLifecycle_VerbsReachTheRuntime(t *testing.T) {
	var calls []string
	record := func(call string) { calls = append(calls, call) }
	a := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return true, nil },
		GetFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
	}
	rt := &mocks.MockRuntime{
		BeginInstallFn: func(context.Context, domain.Namespace, map[string]string) error {
			record("install")
			return nil
		},
		BeginUninstallFn: func(context.Context, domain.Namespace, map[string]string) error {
			record("uninstall")
			return nil
		},
		BeginExecutionFn: func(_ context.Context, _ domain.Namespace, method string, _ map[string]string) error {
			record(method)
			return nil
		},
		BeginStopFn: func(context.Context, domain.Namespace) error {
			record("stop")
			return nil
		},
		ForgetFn: func(context.Context, domain.Namespace) error {
			record("forget")
			return nil
		},
	}
	lc := lifecycle.New(a, rt, &mocks.MockGraph{})
	ctx := context.Background()

	began, err := lc.Install(ctx, row, nil)
	require.NoError(t, err)
	assert.True(t, began)
	require.NoError(t, lc.Uninstall(ctx, row, nil))
	require.NoError(t, lc.Execute(ctx, row, domain.MethodExecute, nil))
	started, err := lc.Update(ctx, row, nil)
	require.NoError(t, err)
	assert.True(t, started, "a row outside the bracket runs the update method")
	require.NoError(t, lc.Stop(ctx, row))
	require.NoError(t, lc.Reset(ctx, row))

	assert.Equal(t, []string{"install", "uninstall", domain.MethodExecute, domain.MethodUpdate, "stop", "forget"}, calls)
}

func TestLifecycle_Recheck_AdvancesARowNothingIsInstalledFrom(t *testing.T) {
	target := domain.Available{Ref: "stable", Commit: "c2"}
	var advanced []domain.Available
	a := &mocks.MockArrow{
		GetFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		CheckAvailableFn: func(context.Context, domain.Namespace) (*domain.Available, error) { return &target, nil },
		AdvanceFn: func(_ context.Context, _ domain.Namespace, got domain.Available) error {
			advanced = append(advanced, got)
			return nil
		},
	}

	_, err := lifecycle.New(a, &mocks.MockRuntime{}, &mocks.MockGraph{}).Recheck(context.Background(), row)

	require.NoError(t, err)
	assert.Equal(t, []domain.Available{target}, advanced)
}

func TestLifecycle_NothingSettling_HoldsNoBadgeAndDrainsAtOnce(t *testing.T) {
	lc := lifecycle.New(&mocks.MockArrow{}, &mocks.MockRuntime{}, &mocks.MockGraph{})

	assert.False(t, lc.Settling(row))
	assert.False(t, lc.HoldBadge(row))
	_, updating := lc.UpdateTarget(row)
	assert.False(t, updating)
	require.NoError(t, lc.Drain(context.Background()))
}

type fakeTrigger struct{ fired []string }

func (f *fakeTrigger) Fire(path string) { f.fired = append(f.fired, path) }
func (f *fakeTrigger) Fired() bool      { return len(f.fired) > 0 }

func TestLifecycle_Activate_AppliesTheStagedBinary(t *testing.T) {
	trig := &fakeTrigger{}
	marked := false
	staged := filepath.Join(t.TempDir(), "quiver-new")
	require.NoError(t, os.WriteFile(staged, []byte("new build"), 0o755))
	size, digest, err := selfupdate.Fingerprint(context.Background(), staged)
	require.NoError(t, err)
	rt := &mocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{Ref: ns, PendingActivation: &domainRuntime.PendingActivation{Version: "v2", Path: staged, Size: size, Digest: digest}}, nil
		},
		MarkActivatingFn: func(context.Context, domain.Namespace) error { marked = true; return nil },
	}
	lc := lifecycle.New(&mocks.MockArrow{}, rt, &mocks.MockGraph{}, lifecycle.WithActivation(trig, "quiver-new"))

	started, err := lc.Activate(context.Background(), row)

	require.NoError(t, err)
	assert.True(t, started)
	assert.True(t, marked)
	assert.Equal(t, []string{staged}, trig.fired)
}

func TestLifecycle_Activate_WithoutAnActivationRefuses(t *testing.T) {
	rt := &mocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{Ref: ns, PendingActivation: &domainRuntime.PendingActivation{Version: "v2", Path: "/p"}}, nil
		},
	}

	started, err := lifecycle.New(&mocks.MockArrow{}, rt, &mocks.MockGraph{}).Activate(context.Background(), row)

	require.ErrorIs(t, err, apperrors.ErrStateViolation)
	assert.False(t, started)
}

func TestLifecycle_Activate_NothingStagedIsANoOp(t *testing.T) {
	rt := &mocks.MockRuntime{}

	started, err := lifecycle.New(&mocks.MockArrow{}, rt, &mocks.MockGraph{}, lifecycle.WithActivation(&fakeTrigger{}, "quiver-new")).Activate(context.Background(), row)

	require.NoError(t, err)
	assert.False(t, started)
}

func TestLifecycle_StagesWhatAnUpdateDeclaredItWouldLeavePending(t *testing.T) {
	workdir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "quiver-new"), []byte("new build"), 0o755))
	var recorded domainRuntime.PendingActivation
	a := &mocks.MockArrow{
		GetFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns, Targets: map[domain.OS]domain.Target{
				domain.CurrentOS(): {Activation: map[string]string{"update": domain.ActivationRestart}},
			}}, nil
		},
	}
	rt := &mocks.MockRuntime{
		RecordPendingActivationFn: func(_ context.Context, _ domain.Namespace, p domainRuntime.PendingActivation) error {
			recorded = p
			return nil
		},
		ReconcileVersionBadgeFn: func(context.Context, domain.Namespace) error { return nil },
	}
	var reaction func(context.Context, domainRuntime.ArrowRuntime)
	rt.OnRuntimeEndedFn = func(fn func(context.Context, domainRuntime.ArrowRuntime)) error {
		reaction = fn
		return nil
	}
	lc := lifecycle.New(a, rt, &mocks.MockGraph{}, lifecycle.WithActivation(&fakeTrigger{}, "quiver-new"))
	require.NoError(t, lc.Start())

	reaction(context.Background(), domainRuntime.ArrowRuntime{
		Ref: row,
		LastReturn: &domainRuntime.Return{
			Method:    domain.MethodUpdate,
			Outcome:   domainRuntime.ExecutionOutcomeSuccess,
			Variables: map[string]string{domain.VarWorkdir: workdir, domain.VarRef: "v2"},
		},
	})
	require.NoError(t, lc.Drain(context.Background()))

	assert.Equal(t, "v2", recorded.Version)
	assert.Equal(t, filepath.Join(workdir, "quiver-new"), recorded.Path)
}
