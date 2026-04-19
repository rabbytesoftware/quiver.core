package deps_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newExecutorService(
	install deps.InstallSyncFunc,
	startFn deps.StartFunc,
	uninstall deps.UninstallSyncFunc,
) deps.Deps {
	return deps.NewTestable(
		deptree.New(),
		makeResolveFunc(nil),
		nil,
		install,
		startFn,
		uninstall,
	)
}

func TestExecute_EmptyPlan_NoOp(t *testing.T) {
	svc := newExecutorService(nil, nil, nil)

	err := svc.Execute(context.Background(), deps.Plan{})

	require.NoError(t, err)
}

func TestExecute_InstallsToolDepsInOrder(t *testing.T) {
	var installed []domain.Namespace
	var started []domain.Namespace

	install := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		installed = append(installed, ns)
		return nil
	}

	startFn := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		started = append(started, ns)
		return nil
	}

	svc := newExecutorService(install, startFn, nil)

	plan := deps.Plan{
		{Namespace: toolNs, Type: domain.ToolDep},
	}

	err := svc.Execute(context.Background(), plan)

	require.NoError(t, err)
	assert.Equal(t, []domain.Namespace{toolNs}, installed)
	assert.Empty(t, started)
}

func TestExecute_StartsServiceDepsAfterInstall(t *testing.T) {
	var installed []domain.Namespace
	var started []domain.Namespace

	install := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		installed = append(installed, ns)
		return nil
	}

	startFn := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		started = append(started, ns)
		return nil
	}

	svc := newExecutorService(install, startFn, nil)

	plan := deps.Plan{
		{Namespace: svcNs, Type: domain.ServiceDep},
	}

	err := svc.Execute(context.Background(), plan)

	require.NoError(t, err)
	assert.Equal(t, []domain.Namespace{svcNs}, installed)
	assert.Equal(t, []domain.Namespace{svcNs}, started)
}

func TestExecute_RollsBackOnFailure(t *testing.T) {
	installErr := errors.New("install failed")
	var uninstalled []domain.Namespace

	callCount := 0
	install := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		callCount++
		if callCount == 2 {
			return installErr
		}
		return nil
	}

	uninstall := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		uninstalled = append(uninstalled, ns)
		return nil
	}

	svc := newExecutorService(install, nil, uninstall)

	plan := deps.Plan{
		{Namespace: toolNs, Type: domain.ToolDep},
		{Namespace: svcNs, Type: domain.ServiceDep},
	}

	err := svc.Execute(context.Background(), plan)

	require.ErrorIs(t, err, installErr)
	assert.Equal(t, []domain.Namespace{toolNs}, uninstalled)
}

func TestExecute_RollbackIgnoresUninstallErrors(t *testing.T) {
	installErr := errors.New("install failed")

	callCount := 0
	install := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		callCount++
		if callCount == 2 {
			return installErr
		}
		return nil
	}

	uninstall := func(
		ctx context.Context,
		ns domain.Namespace,
	) error {
		return errors.New("uninstall also failed")
	}

	svc := newExecutorService(install, nil, uninstall)

	plan := deps.Plan{
		{Namespace: toolNs, Type: domain.ToolDep},
		{Namespace: svcNs, Type: domain.ServiceDep},
	}

	err := svc.Execute(context.Background(), plan)

	require.ErrorIs(t, err, installErr)
}
