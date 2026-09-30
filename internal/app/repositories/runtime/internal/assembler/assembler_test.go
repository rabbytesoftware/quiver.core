package assembler_test

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/assembler"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func testNs() domain.Namespace {
	return domain.Namespace("github.com/user/repo@v1.0.0")
}

func testOs() domain.OS {
	return domain.OSDarwinARM64
}

func newTestAsynxRuntime(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return ax
}

func testArrowWithInstall() *domain.Arrow {
	return &domain.Arrow{
		Namespace: testNs(),
		ArrowMeta: domain.ArrowMeta{Name: "test arrow"},
		Targets: map[domain.OS]domain.Target{
			testOs(): {
				Lifecycle: domain.TargetLifecycle{
					Install: domainStep.StepList{
						domainStep.NewRunStep("install", "echo install", false, "", true),
					},
				},
			},
		},
	}
}

func TestAssemble_Success(t *testing.T) {
	ns := testNs()
	arrow := testArrowWithInstall()

	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		return arrow, nil
	}
	vault := &mocks.Vault{WorkDirValue: "/tmp/workdir"}
	axRuntime := newTestAsynxRuntime(t)

	asm := assembler.New(getArrow, getArrow, axRuntime, vault, nil, testOs())
	result, err := asm.Assemble(context.Background(), ns, domain.MethodInstall, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Steps) // install step + dep step
	assert.Equal(t, "/tmp/workdir", result.WorkDir)
}

func TestAssemble_ArrowNotFound(t *testing.T) {
	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		return nil, asynxModels.ErrNotFound
	}
	axRuntime := newTestAsynxRuntime(t)

	asm := assembler.New(getArrow, getArrow, axRuntime, nil, nil, testOs())
	_, err := asm.Assemble(context.Background(), testNs(), domain.MethodInstall, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestAssemble_GetArrowError(t *testing.T) {
	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		return nil, errors.New("db error")
	}
	axRuntime := newTestAsynxRuntime(t)

	asm := assembler.New(getArrow, getArrow, axRuntime, nil, nil, testOs())
	_, err := asm.Assemble(context.Background(), testNs(), domain.MethodInstall, nil)
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrNotFound)
}

func TestAssemble_PlatformNotSupported(t *testing.T) {
	arrow := &domain.Arrow{
		Namespace: testNs(),
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: domainStep.StepList{
						domainStep.NewRunStep("install", "echo hi", false, "", true),
					},
				},
			},
		},
	}
	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		return arrow, nil
	}
	axRuntime := newTestAsynxRuntime(t)

	// Build for darwin but arrow only has linux target
	asm := assembler.New(getArrow, getArrow, axRuntime, nil, nil, domain.OSDarwinARM64)
	_, err := asm.Assemble(context.Background(), testNs(), domain.MethodInstall, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrPlatformNotSupported)
}

func TestAssemble_MethodNotFound(t *testing.T) {
	arrow := &domain.Arrow{
		Namespace: testNs(),
		Targets: map[domain.OS]domain.Target{
			testOs(): {
				// No methods at all
			},
		},
	}
	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		return arrow, nil
	}
	axRuntime := newTestAsynxRuntime(t)

	asm := assembler.New(getArrow, getArrow, axRuntime, nil, nil, testOs())
	// Execute requires non-empty execute steps
	_, err := asm.Assemble(context.Background(), testNs(), domain.MethodExecute, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrMethodNotFound)
}

func TestAssemble_NilVault_NoWorkDir(t *testing.T) {
	ns := testNs()
	arrow := testArrowWithInstall()

	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		return arrow, nil
	}
	axRuntime := newTestAsynxRuntime(t)

	asm := assembler.New(getArrow, getArrow, axRuntime, nil, nil, testOs())
	result, err := asm.Assemble(context.Background(), ns, domain.MethodInstall, nil)
	require.NoError(t, err)
	assert.Empty(t, result.WorkDir)
	assert.NotEmpty(t, result.Steps)
}

// A workdir the vault cannot give is fatal to every method: steps would
// otherwise run in the daemon's own working directory. A workdir another
// identity owns is a conflict (409), never a server error.
func TestAssemble_VaultWorkDirError_IsFatal(t *testing.T) {
	testCases := []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "collision with another identity", err: vault.ErrWorkDirCollision, wantErr: apperrors.ErrAlreadyExists},
		{name: "any other failure", err: errors.New("disk gone"), wantErr: nil},
	}

	for _, tc := range testCases {
		for _, method := range []string{domain.MethodInstall, domain.MethodUninstall, domain.MethodExecute} {
			t.Run(tc.name+" "+method, func(t *testing.T) {
				arrow := testArrowWithInstall()
				lifecycle := arrow.Targets[testOs()].Lifecycle
				lifecycle.Uninstall = domainStep.StepList{domainStep.NewRunStep("uninstall", "rm -rf data", false, "", true)}
				lifecycle.Execute = domainStep.StepList{domainStep.NewRunStep("execute", "echo run", false, "", true)}
				arrow.Targets[testOs()] = domain.Target{Lifecycle: lifecycle}
				getArrow := func(context.Context, domain.Namespace) (*domain.Arrow, error) { return arrow, nil }
				asm := assembler.New(getArrow, getArrow, newTestAsynxRuntime(t), &mocks.Vault{WorkDirErr: tc.err}, nil, testOs())

				_, err := asm.Assemble(context.Background(), testNs(), method, nil)

				require.ErrorIs(t, err, tc.err)
				if tc.wantErr != nil {
					assert.ErrorIs(t, err, tc.wantErr)
				}
			})
		}
	}
}

func TestAssemble_WithUserVars(t *testing.T) {
	ns := testNs()
	arrow := testArrowWithInstall()

	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		return arrow, nil
	}
	axRuntime := newTestAsynxRuntime(t)

	asm := assembler.New(getArrow, getArrow, axRuntime, nil, nil, testOs())
	result, err := asm.Assemble(context.Background(), ns, domain.MethodInstall, map[string]string{"MY_VAR": "hello"})
	require.NoError(t, err)
	assert.Equal(t, "hello", result.Variables["MY_VAR"])
}

func TestAssemble_Uninstall_AvailableInIsNil(t *testing.T) {
	ns := testNs()
	arrow := &domain.Arrow{
		Namespace: ns,
		Targets: map[domain.OS]domain.Target{
			testOs(): {
				Lifecycle: domain.TargetLifecycle{
					Uninstall: domainStep.StepList{
						domainStep.NewRunStep("uninstall", "echo uninstall", false, "", true),
					},
				},
			},
		},
	}
	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		return arrow, nil
	}
	axRuntime := newTestAsynxRuntime(t)

	asm := assembler.New(getArrow, getArrow, axRuntime, nil, nil, testOs())
	result, err := asm.Assemble(context.Background(), ns, domain.MethodUninstall, nil)
	require.NoError(t, err)
	assert.Nil(t, result.AvailableIn)
}

func TestAssemble_RefVariable(t *testing.T) {
	testCases := []struct {
		name     string
		ns       domain.Namespace
		resolved domain.Resolved
		userVars map[string]string
		opts     []assembler.AssembleOption
		wantRef  string
	}{
		{
			name:     "channel identity takes the resolved ref",
			ns:       "github.com/user/crowbar@stable",
			resolved: domain.Resolved{Ref: "v1.2.0", Commit: "abc"},
			wantRef:  "v1.2.0",
		},
		{
			name:     "target ref wins during an update",
			ns:       "github.com/user/crowbar@stable",
			resolved: domain.Resolved{Ref: "v1.2.0", Commit: "abc"},
			opts:     []assembler.AssembleOption{assembler.WithTargetRef("v1.3.0")},
			wantRef:  "v1.3.0",
		},
		{
			name:    "row with no resolved state falls back to the identity ref",
			ns:      "github.com/user/crowbar@v1.0.0",
			wantRef: "v1.0.0",
		},
		{
			name:    "empty target ref is no override",
			ns:      "github.com/user/crowbar@v1.0.0",
			opts:    []assembler.AssembleOption{assembler.WithTargetRef("")},
			wantRef: "v1.0.0",
		},
		{
			name:     "user supplied REF stays reserved",
			ns:       "github.com/user/crowbar@stable",
			resolved: domain.Resolved{Ref: "v1.2.0"},
			userVars: map[string]string{domain.VarRef: "evil"},
			wantRef:  "v1.2.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			arrow := testArrowWithInstall()
			arrow.Namespace = tc.ns
			arrow.Resolved = tc.resolved
			getArrow := func(context.Context, domain.Namespace) (*domain.Arrow, error) {
				return arrow, nil
			}

			asm := assembler.New(getArrow, getArrow, newTestAsynxRuntime(t), nil, nil, testOs())
			result, err := asm.Assemble(context.Background(), tc.ns, domain.MethodInstall, tc.userVars, tc.opts...)
			require.NoError(t, err)
			assert.Equal(t, tc.wantRef, result.Variables[domain.VarRef])
		})
	}
}
