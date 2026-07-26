package assemblerinternal_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	assemblerinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/assembler/internal"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func newTestAsynxRuntimeForVars(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
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

func testNsForVars() domain.Namespace {
	return domain.Namespace("github.com/user/repo@v1.0.0")
}

func testGetArrow(arrow *domain.Arrow) assemblerinternal.GetArrowFn {
	return func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		if arrow == nil {
			return nil, asynxModels.ErrNotFound
		}
		return arrow, nil
	}
}

func TestResolveVariables_BuiltIns(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{Namespace: ns}
	target := domain.Target{}
	os := domain.OSDarwinARM64
	vault := &mocks.Vault{WorkDirValue: "/tmp/workdir"}
	axRuntime := newTestAsynxRuntimeForVars(t)

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		testGetArrow(arrow),
		axRuntime,
		vault,
		nil,
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "/tmp/workdir", vars["INSTALL_PATH"])
	assert.Equal(t, "/tmp/workdir", vars["WORKDIR"])
	assert.Equal(t, ns.String(), vars["ARROW_NAMESPACE"])
	assert.Equal(t, os.String(), vars["PLATFORM"])
	assert.Equal(t, "v1.0.0", vars["REF"])
}

func TestResolveVariables_Ref(t *testing.T) {
	testCases := []struct {
		name        string
		ns          domain.Namespace
		expectedRef string
	}{
		{
			name:        "tag ref",
			ns:          domain.Namespace("github.com/user/repo@v1.2.0"),
			expectedRef: "v1.2.0",
		},
		{
			name:        "branch ref",
			ns:          domain.Namespace("github.com/user/repo@main"),
			expectedRef: "main",
		},
		{
			name:        "refless namespace yields empty ref",
			ns:          domain.Namespace("github.com/user/repo"),
			expectedRef: "",
		},
		{
			name:        "quiver-hosted namespace with ref",
			ns:          domain.Namespace("github.com/user/repo/auid@v3"),
			expectedRef: "v3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			arrow := &domain.Arrow{Namespace: tc.ns}
			axRuntime := newTestAsynxRuntimeForVars(t)

			vars, err := assemblerinternal.ResolveVariables(
				context.Background(),
				tc.ns,
				arrow,
				domain.Target{},
				domain.OSDarwinARM64,
				testGetArrow(arrow),
				axRuntime,
				nil,
				nil,
				nil,
			)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedRef, vars["REF"])
		})
	}
}

func TestResolveVariables_NilVault_NoInstallPath(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{Namespace: ns}
	target := domain.Target{}
	os := domain.OSDarwinARM64
	axRuntime := newTestAsynxRuntimeForVars(t)

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		testGetArrow(arrow),
		axRuntime,
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	_, hasInstallPath := vars["INSTALL_PATH"]
	assert.False(t, hasInstallPath)
	assert.Equal(t, ns.String(), vars["ARROW_NAMESPACE"])
}

func TestResolveVariables_WithDefaults(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{
		Namespace: ns,
		Variables: []domain.Variable{
			{Name: "DB_HOST", Default: "localhost"},
			{Name: "DB_PORT", Default: "5432"},
		},
	}
	target := domain.Target{}
	os := domain.OSDarwinARM64
	axRuntime := newTestAsynxRuntimeForVars(t)

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		testGetArrow(arrow),
		axRuntime,
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "localhost", vars["DB_HOST"])
	assert.Equal(t, "5432", vars["DB_PORT"])
}

func TestResolveVariables_UserVarsOverrideDefaults(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{
		Namespace: ns,
		Variables: []domain.Variable{
			{Name: "DB_HOST", Default: "localhost"},
		},
	}
	target := domain.Target{}
	os := domain.OSDarwinARM64
	axRuntime := newTestAsynxRuntimeForVars(t)

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		testGetArrow(arrow),
		axRuntime,
		nil,
		nil,
		map[string]string{"DB_HOST": "production.db"},
	)
	require.NoError(t, err)
	assert.Equal(t, "production.db", vars["DB_HOST"])
}

func TestResolveVariables_MissingRequired_Error(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{
		Namespace: ns,
		Variables: []domain.Variable{
			{Name: "REQUIRED_VAR", Default: ""}, // no default = required
		},
	}
	target := domain.Target{}
	os := domain.OSDarwinARM64
	axRuntime := newTestAsynxRuntimeForVars(t)

	_, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		testGetArrow(arrow),
		axRuntime,
		nil,
		nil,
		nil, // no user vars
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrMissingVariable)
}

func TestResolveVariables_MissingRequired_ProvidedByUser(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{
		Namespace: ns,
		Variables: []domain.Variable{
			{Name: "REQUIRED_VAR", Default: ""},
		},
	}
	target := domain.Target{}
	os := domain.OSDarwinARM64
	axRuntime := newTestAsynxRuntimeForVars(t)

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		testGetArrow(arrow),
		axRuntime,
		nil,
		nil,
		map[string]string{"REQUIRED_VAR": "value"},
	)
	require.NoError(t, err)
	assert.Equal(t, "value", vars["REQUIRED_VAR"])
}

func TestResolveVariables_StoredVarsFromLastReturn(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{Namespace: ns}
	target := domain.Target{}
	os := domain.OSDarwinARM64
	axRuntime := newTestAsynxRuntimeForVars(t)

	// Seed a runtime with a previous execution that stored vars
	// We need to directly set a state with LastReturn
	cmdImpl := &setStoredVarsCommand{ns: ns, storedVars: map[string]string{"STORED_VAR": "stored_value"}}
	_, err := axRuntime.Send(context.Background(), cmdImpl)
	require.NoError(t, err)

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		testGetArrow(arrow),
		axRuntime,
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "stored_value", vars["STORED_VAR"])
}

func TestResolveVariables_DepBuiltIns_WithVault(t *testing.T) {
	ns := testNsForVars()
	depNs := domain.Namespace("github.com/user/dep")
	os := domain.OSDarwinARM64

	depArrow := &domain.Arrow{
		Namespace: depNs,
		Targets:   map[domain.OS]domain.Target{os: {Exports: map[string]string{"bin": "/bin/dep"}}},
	}
	arrow := &domain.Arrow{
		Namespace: ns,
		Targets: map[domain.OS]domain.Target{
			os: {Tools: []domain.DependencyEdge{{Namespace: depNs}}},
		},
	}

	target := arrow.Targets[os]
	vault := &mocks.Vault{WorkDirValue: "/workdir"}
	axRuntime := newTestAsynxRuntimeForVars(t)

	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		if n == depNs {
			return depArrow, nil
		}
		return arrow, nil
	}

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		getArrow,
		axRuntime,
		vault,
		nil,
		nil,
	)
	require.NoError(t, err)
	// Dep exports should be available
	assert.Equal(t, "/bin/dep", vars[depNs.String()+".bin"])
}

func TestResolveVariables_DepNotFound_Skipped(t *testing.T) {
	ns := testNsForVars()
	depNs := domain.Namespace("github.com/user/dep")
	os := domain.OSDarwinARM64

	arrow := &domain.Arrow{
		Namespace: ns,
		Targets: map[domain.OS]domain.Target{
			os: {Tools: []domain.DependencyEdge{{Namespace: depNs}}},
		},
	}
	target := arrow.Targets[os]
	axRuntime := newTestAsynxRuntimeForVars(t)

	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		if n == depNs {
			return nil, asynxModels.ErrNotFound
		}
		return arrow, nil
	}

	// Should not error - missing dep is skipped
	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		getArrow,
		axRuntime,
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	assert.NotNil(t, vars)
}

func TestResolveVariables_GetArrowUnexpectedError_Logged(t *testing.T) {
	ns := testNsForVars()
	depNs := domain.Namespace("github.com/user/dep")
	os := domain.OSDarwinARM64

	arrow := &domain.Arrow{
		Namespace: ns,
		Targets: map[domain.OS]domain.Target{
			os: {Tools: []domain.DependencyEdge{{Namespace: depNs}}},
		},
	}
	target := arrow.Targets[os]
	axRuntime := newTestAsynxRuntimeForVars(t)

	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		if n == depNs {
			return nil, errors.New("unexpected db error")
		}
		return arrow, nil
	}

	// Unexpected errors are logged but not returned
	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		getArrow,
		axRuntime,
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	assert.NotNil(t, vars)
}

func TestResolveVariables_Netbridge_SuccessfulAllocation(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{
		Namespace: ns,
		Netbridge: []netbridge.PortDef{
			{Name: "API_PORT", Protocol: netbridge.ProtocolTCP, Default: 8080, Required: false},
		},
	}
	target := domain.Target{}
	os := domain.OSDarwinARM64
	axRuntime := newTestAsynxRuntimeForVars(t)
	nb := &mocks.Netbridge{AllocatePort: 9000}

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		testGetArrow(arrow),
		axRuntime,
		nil,
		nb,
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "9000", vars["API_PORT"])
}

func TestResolveVariables_Netbridge_RequiredAllocationError(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{
		Namespace: ns,
		Netbridge: []netbridge.PortDef{
			{Name: "API_PORT", Protocol: "tcp", Default: 8080, Required: true},
		},
	}
	target := domain.Target{}
	os := domain.OSDarwinARM64
	axRuntime := newTestAsynxRuntimeForVars(t)
	nb := &mocks.Netbridge{AllocateErr: errors.New("port unavailable")}

	_, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		testGetArrow(arrow),
		axRuntime,
		nil,
		nb,
		nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "port unavailable")
}

func TestResolveVariables_Netbridge_OptionalAllocationError_Skipped(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{
		Namespace: ns,
		Netbridge: []netbridge.PortDef{
			{Name: "API_PORT", Protocol: netbridge.ProtocolTCP, Default: 8080, Required: false},
		},
	}
	target := domain.Target{}
	os := domain.OSDarwinARM64
	axRuntime := newTestAsynxRuntimeForVars(t)
	nb := &mocks.Netbridge{AllocateErr: errors.New("port unavailable")}

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		testGetArrow(arrow),
		axRuntime,
		nil,
		nb,
		nil,
	)
	require.NoError(t, err)
	_, hasPort := vars["API_PORT"]
	assert.False(t, hasPort)
}

func TestResolveVariables_DepExport_RelativePath_WithInstallPath(t *testing.T) {
	ns := testNsForVars()
	depFullNs := domain.Namespace("github.com/user/dep@v1")
	depBareNs := domain.Namespace("github.com/user/dep")
	os := domain.OSDarwinARM64

	depArrow := &domain.Arrow{
		Namespace: depFullNs,
		Targets:   map[domain.OS]domain.Target{os: {Exports: map[string]string{"bin": "./bin/dep"}}},
	}
	arrow := &domain.Arrow{
		Namespace: ns,
		Targets: map[domain.OS]domain.Target{
			os: {Tools: []domain.DependencyEdge{{Namespace: depFullNs}}},
		},
	}

	target := arrow.Targets[os]
	vault := &mocks.Vault{WorkDirValue: "/home/user/quiver"}
	axRuntime := newTestAsynxRuntimeForVars(t)

	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		if n == depFullNs {
			return depArrow, nil
		}
		return arrow, nil
	}

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		getArrow,
		axRuntime,
		vault,
		nil,
		nil,
	)
	require.NoError(t, err)
	// Relative path should be anchored to dep's INSTALL_PATH (using bare namespace)
	assert.Equal(t, filepath.Join("/home/user/quiver", "bin/dep"), vars[depBareNs.String()+".bin"])
}

func TestResolveVariables_DepNoTarget_Skipped(t *testing.T) {
	ns := testNsForVars()
	depNs := domain.Namespace("github.com/user/dep@v1")
	os := domain.OSDarwinARM64
	otherOS := domain.OSLinuxAMD64

	depArrow := &domain.Arrow{
		Namespace: depNs,
		// Target for otherOS, not os
		Targets: map[domain.OS]domain.Target{otherOS: {Exports: map[string]string{"bin": "/bin/dep"}}},
	}
	arrow := &domain.Arrow{
		Namespace: ns,
		Targets: map[domain.OS]domain.Target{
			os: {Tools: []domain.DependencyEdge{{Namespace: depNs}}},
		},
	}

	target := arrow.Targets[os]
	axRuntime := newTestAsynxRuntimeForVars(t)

	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		if n == depNs {
			return depArrow, nil
		}
		return arrow, nil
	}

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		getArrow,
		axRuntime,
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	// Dep exports for missing OS should be skipped
	_, hasExport := vars[depNs.String()+".bin"]
	assert.False(t, hasExport)
}

func TestResolveVariables_VaultWorkDirError_Skipped(t *testing.T) {
	ns := testNsForVars()
	depFullNs := domain.Namespace("github.com/user/dep@v1")
	depBareNs := domain.Namespace("github.com/user/dep")
	os := domain.OSDarwinARM64

	depArrow := &domain.Arrow{
		Namespace: depFullNs,
		Targets:   map[domain.OS]domain.Target{os: {Exports: map[string]string{"bin": "./bin/dep"}}},
	}
	arrow := &domain.Arrow{
		Namespace: ns,
		Targets: map[domain.OS]domain.Target{
			os: {Tools: []domain.DependencyEdge{{Namespace: depFullNs}}},
		},
	}

	target := arrow.Targets[os]
	// Vault that fails on WorkDir
	vault := &mocks.Vault{WorkDirErr: errors.New("vault unavailable")}
	axRuntime := newTestAsynxRuntimeForVars(t)

	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		if n == depFullNs {
			return depArrow, nil
		}
		return arrow, nil
	}

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		target,
		os,
		getArrow,
		axRuntime,
		vault,
		nil,
		nil,
	)
	require.NoError(t, err)
	// Dep INSTALL_PATH should not be set due to vault error
	_, hasInstallPath := vars[depBareNs.String()+".INSTALL_PATH"]
	assert.False(t, hasInstallPath)
	// But relative path export should be treated as-is without anchoring
	assert.Equal(t, "./bin/dep", vars[depBareNs.String()+".bin"])
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

type setStoredVarsCommand struct {
	ns         domain.Namespace
	storedVars map[string]string
}

func (c *setStoredVarsCommand) AggregateID() string  { return c.ns.String() }
func (c *setStoredVarsCommand) EventName() string    { return "runtime.vars_set." + c.ns.String() }
func (c *setStoredVarsCommand) ShouldSnapshot() bool { return true }
func (c *setStoredVarsCommand) Validate(_ *domainRuntime.ArrowRuntime) error {
	return nil
}

func (c *setStoredVarsCommand) EmitEvent(_ *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:   c.ns,
		State: domain.ArrowStateReady,
		LastReturn: &domainRuntime.Return{
			Method:    domain.MethodInstall,
			Outcome:   domainRuntime.ExecutionOutcomeSuccess,
			Variables: c.storedVars,
		},
	}
}

// The boundary rejects reserved names, but the assembler must not depend on
// that: a built-in reaching it directly still loses to the computed value.
func TestResolveVariables_ReservedUserVars_KeepTheComputedValue(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{Namespace: ns}
	os := domain.OSDarwinARM64
	vault := &mocks.Vault{WorkDirValue: "/tmp/workdir"}
	axRuntime := newTestAsynxRuntimeForVars(t)

	userVars := make(map[string]string, len(domain.ReservedVariableNames()))
	for _, name := range domain.ReservedVariableNames() {
		userVars[name] = "hijacked"
	}
	userVars["PORT"] = "8080"

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		domain.Target{},
		os,
		testGetArrow(arrow),
		axRuntime,
		vault,
		nil,
		userVars,
	)
	require.NoError(t, err)

	assert.Equal(t, "/tmp/workdir", vars[domain.VarWorkdir])
	assert.Equal(t, "/tmp/workdir", vars[domain.VarInstallPath])
	assert.Equal(t, ns.String(), vars[domain.VarArrowNamespace])
	assert.Equal(t, os.String(), vars[domain.VarPlatform])
	assert.Equal(t, "v1.0.0", vars[domain.VarRef])
	assert.Equal(t, "8080", vars["PORT"])
}

// A reserved name is dropped, not turned into an error and not left unset: the
// computed value stands even when the vault could not supply a workdir.
func TestResolveVariables_ReservedUserVar_WithoutVault_IsStillDropped(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{Namespace: ns}
	axRuntime := newTestAsynxRuntimeForVars(t)

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		domain.Target{},
		domain.OSDarwinARM64,
		testGetArrow(arrow),
		axRuntime,
		nil,
		nil,
		map[string]string{domain.VarWorkdir: "/etc"},
	)
	require.NoError(t, err)

	assert.NotContains(t, vars, domain.VarWorkdir)
}
