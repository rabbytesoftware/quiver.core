package runtime_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	runtimeMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func updatableArrow(ns domain.Namespace, available *domain.Available) *domain.Arrow {
	return &domain.Arrow{
		Namespace: ns,
		Resolved:  domain.Resolved{Ref: "v1.2.0", Commit: "old"},
		Available: available,
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinARM64: {
				Lifecycle: domain.TargetLifecycle{
					Update: domainStep.StepList{
						domainStep.NewRunStep("update", "echo ${REF}", false, "", true),
					},
				},
			},
		},
	}
}

// ${REF} during an update names the target the update is moving to, not the
// installed ref and never the channel selector in the identity.
func TestBeginUpdate_RefVariable(t *testing.T) {
	testCases := []struct {
		name      string
		available *domain.Available
		wantRef   string
	}{
		{name: "target ref from available", available: &domain.Available{Ref: "v1.3.0", Commit: "new"}, wantRef: "v1.3.0"},
		{name: "no available falls back to resolved", available: nil, wantRef: "v1.2.0"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ax := newTestAsynxRuntime(t)
			ns := domain.Namespace("github.com/user/crowbar@stable")
			arrow := updatableArrow(ns, tc.available)
			getArrow := func(context.Context, domain.Namespace) (*domain.Arrow, error) { return arrow, nil }
			f := catToFuncs(&runtimeMocks.MockArrow{})
			repo, err := runtime.New(getArrow, getArrow, ax, nil, nil, f.markInstalled, f.markUninstalled, f.markLastUsed, f.hasDependents, f.listArrows, domain.OSDarwinARM64, func(context.Context) ([]domain.Namespace, error) { return nil, nil })
			require.NoError(t, err)
			seedReadyRuntime(t, ax, ns)

			require.NoError(t, repo.BeginUpdate(context.Background(), ns, nil))

			got, err := ax.Get(context.Background(), ns.String())
			require.NoError(t, err)
			require.NotNil(t, got.Execution)
			assert.Equal(t, tc.wantRef, got.Execution.Variables[domain.VarRef])
		})
	}
}

// A catalog read failure is the assembler's to report; BeginUpdate does not
// swallow it into a ref-less update.
func TestBeginUpdate_CatalogReadFails_ReturnsError(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := domain.Namespace("github.com/user/crowbar@stable")
	getArrow := func(context.Context, domain.Namespace) (*domain.Arrow, error) { return nil, assert.AnError }
	f := catToFuncs(&runtimeMocks.MockArrow{})
	repo, err := runtime.New(getArrow, getArrow, ax, nil, nil, f.markInstalled, f.markUninstalled, f.markLastUsed, f.hasDependents, f.listArrows, domain.OSDarwinARM64, func(context.Context) ([]domain.Namespace, error) { return nil, nil })
	require.NoError(t, err)
	seedReadyRuntime(t, ax, ns)

	require.ErrorIs(t, repo.BeginUpdate(context.Background(), ns, nil), assert.AnError)
}
