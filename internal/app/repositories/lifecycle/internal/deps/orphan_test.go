package deps

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

const (
	declaredCommitDep   = domain.Namespace("github.com/user/dep@ABCDEF1")
	cataloguedCommitDep = domain.Namespace("github.com/user/dep@abcdef1")
)

// orphanFixture catalogues a commit dependency in lower case while its
// dependent declares it in upper case, and records every uninstall.
func orphanFixture(parent domain.Namespace, pending *domainRuntime.DepSyncInfo) (*harness, *[]domain.Namespace) {
	var uninstalled []domain.Namespace
	a := &mocks.MockArrow{
		ResolveCataloguedFn: func(_ context.Context, ns domain.Namespace) (domain.Namespace, error) {
			if ns == declaredCommitDep {
				return cataloguedCommitDep, nil
			}
			return ns, nil
		},
		GetFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(_ context.Context, ns domain.Namespace) (domain.ArrowState, error) {
			if ns == cataloguedCommitDep {
				return domain.ArrowStateReady, nil
			}
			return "", nil
		},
		GetRuntimeFn: func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateOutdated, PendingDepSync: pending}, nil
		},
		BeginUninstallFn: func(_ context.Context, ns domain.Namespace, _ map[string]string) error {
			uninstalled = append(uninstalled, ns)
			return nil
		},
	}
	g := &mocks.MockGraph{
		ResolveFn: func(_ context.Context, ns domain.Namespace) (models.Plan, error) {
			if ns == parent {
				return models.Plan{{Namespace: declaredCommitDep, Type: domain.ToolDep}}, nil
			}
			return nil, nil
		},
	}
	return newUC(a, rt, g), &uninstalled
}

// A dependency a target dropped, declared with its commit in another case
// than the catalog holds, is still uninstalled as an orphan.
func TestRuntimeSyncDeps_RemovedCommitDepInAnotherCase_IsUninstalled(t *testing.T) {
	uc, uninstalled := orphanFixture(rollingRow, &domainRuntime.DepSyncInfo{RemovedDeps: []domain.Namespace{declaredCommitDep}})

	require.NoError(t, uc.syncDeps(context.Background(), rollingRow))

	assert.Equal(t, []domain.Namespace{cataloguedCommitDep}, *uninstalled)
}

// Uninstalling a dependent uninstalls a commit dependency it declared in
// another case than the catalog holds.
func TestRuntimeOnUninstallEnded_CommitDepInAnotherCase_IsUninstalled(t *testing.T) {
	uc, uninstalled := orphanFixture(rollingRow, nil)

	uc.onUninstallEnded(context.Background(), domainRuntime.ArrowRuntime{Ref: rollingRow})

	assert.Equal(t, []domain.Namespace{cataloguedCommitDep}, *uninstalled)
}

// A declared dependency the catalog holds no row for has nothing to retire.
func TestRuntimeOrphanChecks_UncataloguedDep_RetireNothing(t *testing.T) {
	testCases := []struct {
		name string
		run  func(*harness)
	}{
		{name: "dropped by a target", run: func(uc *harness) {
			require.NoError(t, uc.syncDeps(context.Background(), rollingRow))
		}},
		{name: "dependent uninstalled", run: func(uc *harness) {
			uc.onUninstallEnded(context.Background(), domainRuntime.ArrowRuntime{Ref: rollingRow})
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			uc, uninstalled := orphanFixture(rollingRow, &domainRuntime.DepSyncInfo{RemovedDeps: []domain.Namespace{declaredCommitDep}})
			uc.arrow.(*mocks.MockArrow).ResolveCataloguedFn = func(context.Context, domain.Namespace) (domain.Namespace, error) {
				return "", apperrors.ErrNotFound
			}

			tc.run(uc)

			assert.Empty(t, *uninstalled)
		})
	}
}
