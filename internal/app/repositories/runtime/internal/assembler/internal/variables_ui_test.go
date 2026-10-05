package assemblerinternal_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	assemblerinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/assembler/internal"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

type fakeSockets struct {
	path  string
	err   error
	asked []domain.Namespace
}

func (f *fakeSockets) Prepare(ns domain.Namespace) (string, error) {
	f.asked = append(f.asked, ns)
	return f.path, f.err
}

func resolveUI(
	t *testing.T,
	socks assemblerinternal.SocketProvider,
	steps []domainStep.Step,
	userVars map[string]string,
) (map[string]string, error) {
	t.Helper()
	ns := testNsForVars()
	arrow := &domain.Arrow{Namespace: ns}
	return assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		domain.Target{},
		domain.OSDarwinARM64,
		testGetArrow(arrow),
		newTestAsynxRuntimeForVars(t),
		nil,
		nil,
		socks,
		userVars,
		steps,
	)
}

func resolveWithSockets(t *testing.T, socks assemblerinternal.SocketProvider, steps []domainStep.Step) map[string]string {
	t.Helper()
	return resolveWithUserVars(t, socks, steps, nil)
}

func resolveWithUserVars(
	t *testing.T,
	socks assemblerinternal.SocketProvider,
	steps []domainStep.Step,
	userVars map[string]string,
) map[string]string {
	t.Helper()
	vars, err := resolveUI(t, socks, steps, userVars)
	require.NoError(t, err)
	return vars
}

func resolveErr(t *testing.T, socks assemblerinternal.SocketProvider, steps []domainStep.Step) error {
	t.Helper()
	_, err := resolveUI(t, socks, steps, nil)
	return err
}

func TestResolveVariables_ProvisionsArrowUIListen(t *testing.T) {
	socks := &fakeSockets{path: "/run/abc.sock"}
	steps := []domainStep.Step{domainStep.NewUIStep("Chat", []string{"unix"}, "", "", true)}

	vars := resolveWithSockets(t, socks, steps)

	require.Equal(t, "/run/abc.sock", vars[domain.VarArrowUIListen])
	require.Equal(t, []domain.Namespace{testNsForVars()}, socks.asked)
}

func TestResolveVariables_NoListenStepNoSocket(t *testing.T) {
	socks := &fakeSockets{path: "/run/abc.sock"}
	steps := []domainStep.Step{domainStep.NewUIStep("Docs", nil, "./dist", "", true)}

	vars := resolveWithSockets(t, socks, steps)

	require.NotContains(t, vars, domain.VarArrowUIListen)
	require.Empty(t, socks.asked)
}

func TestResolveVariables_NoSocketProviderNoSocket(t *testing.T) {
	steps := []domainStep.Step{domainStep.NewUIStep("Chat", []string{"unix"}, "", "", true)}

	vars := resolveWithSockets(t, nil, steps)

	require.NotContains(t, vars, domain.VarArrowUIListen)
}

func TestResolveVariables_UserCannotOverrideArrowUIListen(t *testing.T) {
	socks := &fakeSockets{path: "/run/abc.sock"}
	steps := []domainStep.Step{domainStep.NewUIStep("Chat", []string{"unix"}, "", "", true)}

	vars := resolveWithUserVars(t, socks, steps, map[string]string{domain.VarArrowUIListen: "/evil"})

	require.Equal(t, "/run/abc.sock", vars[domain.VarArrowUIListen])
}

func TestResolveVariables_PrepareErrorPropagates(t *testing.T) {
	prepareErr := errors.New("path too long")
	socks := &fakeSockets{err: prepareErr}
	steps := []domainStep.Step{domainStep.NewUIStep("Chat", []string{"unix"}, "", "", true)}

	require.ErrorIs(t, resolveErr(t, socks, steps), prepareErr)
}
