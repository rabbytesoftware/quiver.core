package assembler_test

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/runtime/internal/assembler"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
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

	asm := assembler.New(getArrow, axRuntime, vault, nil, testOs())
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

	asm := assembler.New(getArrow, axRuntime, nil, nil, testOs())
	_, err := asm.Assemble(context.Background(), testNs(), domain.MethodInstall, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestAssemble_GetArrowError(t *testing.T) {
	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		return nil, errors.New("db error")
	}
	axRuntime := newTestAsynxRuntime(t)

	asm := assembler.New(getArrow, axRuntime, nil, nil, testOs())
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
	asm := assembler.New(getArrow, axRuntime, nil, nil, domain.OSDarwinARM64)
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

	asm := assembler.New(getArrow, axRuntime, nil, nil, testOs())
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

	asm := assembler.New(getArrow, axRuntime, nil, nil, testOs())
	result, err := asm.Assemble(context.Background(), ns, domain.MethodInstall, nil)
	require.NoError(t, err)
	assert.Empty(t, result.WorkDir)
	assert.NotEmpty(t, result.Steps)
}

func TestAssemble_VaultWorkDirError_NonFatal(t *testing.T) {
	ns := testNs()
	arrow := testArrowWithInstall()

	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		return arrow, nil
	}
	vault := &mocks.Vault{WorkDirErr: errors.New("vault error")}
	axRuntime := newTestAsynxRuntime(t)

	asm := assembler.New(getArrow, axRuntime, vault, nil, testOs())
	result, err := asm.Assemble(context.Background(), ns, domain.MethodInstall, nil)
	require.NoError(t, err)
	assert.Empty(t, result.WorkDir)
}

func TestAssemble_WithUserVars(t *testing.T) {
	ns := testNs()
	arrow := testArrowWithInstall()

	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		return arrow, nil
	}
	axRuntime := newTestAsynxRuntime(t)

	asm := assembler.New(getArrow, axRuntime, nil, nil, testOs())
	result, err := asm.Assemble(context.Background(), ns, domain.MethodInstall, map[string]string{"MY_VAR": "hello"})
	require.NoError(t, err)
	assert.Equal(t, "hello", result.Variables["MY_VAR"])
}

func TestAssemble_AvailableInSetForUninstall(t *testing.T) {
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

	asm := assembler.New(getArrow, axRuntime, nil, nil, testOs())
	result, err := asm.Assemble(context.Background(), ns, domain.MethodUninstall, nil)
	require.NoError(t, err)
	assert.Contains(t, result.AvailableIn, domain.ArrowStateReady)
}
