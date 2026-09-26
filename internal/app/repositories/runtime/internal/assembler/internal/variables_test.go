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
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func newTestAsynxRuntimeForVars(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
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
		stepsUnderTest(arrow),
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
				stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
	)
	require.NoError(t, err)
	// Relative path should be anchored to dep's INSTALL_PATH (using bare namespace)
	assert.Equal(t, filepath.Join("/home/user/quiver", "bin/dep"), vars[depBareNs.String()+".bin"])
}

func TestResolveVariables_DepExport_BareEdgeNamespace_WorkDirUsesResolvedDepNamespace(t *testing.T) {
	ns := testNsForVars()
	depBareNs := domain.Namespace("github.com/rabbytesoftware/quiver.essentials/appimage-runtime")
	depResolvedNs := domain.Namespace("github.com/rabbytesoftware/quiver.essentials/appimage-runtime@master")
	os := domain.OSDarwinARM64

	depArrow := &domain.Arrow{
		Namespace: depResolvedNs,
		Targets:   map[domain.OS]domain.Target{os: {Exports: map[string]string{"extract": "./appimage-extract.sh"}}},
	}
	arrow := &domain.Arrow{
		Namespace: ns,
		Targets: map[domain.OS]domain.Target{
			os: {Tools: []domain.DependencyEdge{{Namespace: depBareNs}}},
		},
	}

	target := arrow.Targets[os]
	vault := &mocks.Vault{WorkDirValue: "/home/user/.quiver/namespaces/github.com/rabbytesoftware/quiver.essentials/appimage-runtime@master"}
	axRuntime := newTestAsynxRuntimeForVars(t)

	getArrow := func(ctx context.Context, n domain.Namespace) (*domain.Arrow, error) {
		if n == depBareNs {
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
		stepsUnderTest(arrow),
	)
	require.NoError(t, err)
	require.Len(t, vault.WorkDirNamespaces, 2)
	assert.Equal(t, depResolvedNs, vault.WorkDirNamespaces[1])
	assert.Equal(t, filepath.Join(vault.WorkDirValue, "appimage-extract.sh"), vars[depBareNs.String()+".extract"])
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
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
		stepsUnderTest(arrow),
	)
	require.NoError(t, err)

	assert.NotContains(t, vars, domain.VarWorkdir)
}

// stepsUnderTest gives a test the step list ResolveVariables now scans to
// decide what is required. It references every variable the arrow declares,
// which is what these tests assumed implicitly before required-ness became
// per-method: each one is about the LAYERS that resolve a value, not about
// which lifecycle happens to read it.
func stepsUnderTest(arrow *domain.Arrow) []domainStep.Step {
	if arrow == nil {
		return nil
	}
	command := ""
	for _, v := range arrow.Variables {
		command += "${" + v.Name + "} "
	}
	return []domainStep.Step{
		domainStep.NewRunStep("every declared variable", command, false, "10s", true),
	}
}

// TestResolveVariables_UnreferencedRequiredVar_NotDemanded is the C1 case, in
// the shape both self-manifests actually hit: an arrow declares a no-default
// variable for its install and update lifecycles, and a method that never
// expands it (uninstall) is run with no variables at all. That used to be
// refused, which is what made quiver.desktop's uninstall and update buttons
// -- neither of which sends variables -- unreachable from the real app, and
// what made quiver.core's own self-arrow fail to install as a dependency of
// quiver.desktop, since installOneDep passes nil.
func TestResolveVariables_UnreferencedRequiredVar_NotDemanded(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{
		Namespace: ns,
		Variables: []domain.Variable{
			{Name: "QUIVER_RELEASE_ASSET_URL", Default: ""},
			{Name: "APPIMAGE_PATH", Default: "/opt/Quiver.AppImage"},
		},
	}
	uninstallSteps := []domainStep.Step{
		domainStep.NewRunStep("remove it", `rm -f "${APPIMAGE_PATH}"`, false, "2m", true),
	}

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		domain.Target{},
		domain.OSLinuxAMD64,
		testGetArrow(arrow),
		newTestAsynxRuntimeForVars(t),
		nil,
		nil,
		nil, // exactly what the desktop UI sends on uninstall
		uninstallSteps,
	)

	require.NoError(t, err)
	assert.Equal(t, "/opt/Quiver.AppImage", vars["APPIMAGE_PATH"])
}

// TestResolveVariables_ReferencedRequiredVar_StillDemanded is the other half,
// and the one that must not regress: a step that expands a variable with no
// value has to be refused by name, not left to expand to an empty URL and
// fetch nothing.
func TestResolveVariables_ReferencedRequiredVar_StillDemanded(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{
		Namespace: ns,
		Variables: []domain.Variable{{Name: "QUIVER_RELEASE_ASSET_URL", Default: ""}},
	}
	updateSteps := []domainStep.Step{
		domainStep.NewFetchStep("download", "${QUIVER_RELEASE_ASSET_URL}", "./x", "", "10m", true),
	}

	_, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		domain.Target{},
		domain.OSLinuxAMD64,
		testGetArrow(arrow),
		newTestAsynxRuntimeForVars(t),
		nil,
		nil,
		nil,
		updateSteps,
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrMissingVariable)
	assert.Contains(t, err.Error(), "QUIVER_RELEASE_ASSET_URL")
}

// TestResolveVariables_ReferencedRequiredVar_SuppliedByCaller closes the
// triangle: supplied, and the execution proceeds.
func TestResolveVariables_ReferencedRequiredVar_SuppliedByCaller(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{
		Namespace: ns,
		Variables: []domain.Variable{{Name: "QUIVER_RELEASE_ASSET_URL", Default: ""}},
	}
	updateSteps := []domainStep.Step{
		domainStep.NewFetchStep("download", "${QUIVER_RELEASE_ASSET_URL}", "./x", "", "10m", true),
	}

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		domain.Target{},
		domain.OSLinuxAMD64,
		testGetArrow(arrow),
		newTestAsynxRuntimeForVars(t),
		nil,
		nil,
		map[string]string{"QUIVER_RELEASE_ASSET_URL": "https://example.invalid/asset"},
		updateSteps,
	)

	require.NoError(t, err)
	assert.Equal(t, "https://example.invalid/asset", vars["QUIVER_RELEASE_ASSET_URL"])
}

// TestResolveVariables_RequiredVar_NotCarriedForward is the stale-value
// hazard, asserted at its source. An arrow updated once with a release URL
// must not be updatable a second time with no URL at all: layer 5 used to
// copy the previous execution's whole variable map forward, which satisfied
// the required-variable check without anyone having supplied anything and
// re-downloaded the asset from the LAST update -- the version the user
// already has. The failure has to be loud.
func TestResolveVariables_RequiredVar_NotCarriedForward(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{
		Namespace: ns,
		Variables: []domain.Variable{{Name: "QUIVER_RELEASE_ASSET_URL", Default: ""}},
	}
	updateSteps := []domainStep.Step{
		domainStep.NewFetchStep("download", "${QUIVER_RELEASE_ASSET_URL}", "./x", "", "10m", true),
	}
	axRuntime := newTestAsynxRuntimeForVars(t)

	// The first update happened, and recorded the URL it ran with.
	_, err := axRuntime.Send(context.Background(), &setStoredVarsCommand{
		ns:         ns,
		storedVars: map[string]string{"QUIVER_RELEASE_ASSET_URL": "https://example.test/OLD.AppImage"},
	})
	require.NoError(t, err)

	// A second update, named by nobody.
	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		domain.Target{},
		domain.OSLinuxAMD64,
		testGetArrow(arrow),
		axRuntime,
		nil,
		nil,
		nil,
		updateSteps,
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrMissingVariable)
	assert.Contains(t, err.Error(), "QUIVER_RELEASE_ASSET_URL")
	assert.Nil(t, vars)
}

// TestResolveVariables_RequiredVar_CallerValueWinsOverStored is the same
// situation with the caller doing its job: a fresh value is used, and the
// stored one is not merely outranked but never considered.
func TestResolveVariables_RequiredVar_CallerValueWinsOverStored(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{
		Namespace: ns,
		Variables: []domain.Variable{{Name: "QUIVER_RELEASE_ASSET_URL", Default: ""}},
	}
	updateSteps := []domainStep.Step{
		domainStep.NewFetchStep("download", "${QUIVER_RELEASE_ASSET_URL}", "./x", "", "10m", true),
	}
	axRuntime := newTestAsynxRuntimeForVars(t)

	_, err := axRuntime.Send(context.Background(), &setStoredVarsCommand{
		ns:         ns,
		storedVars: map[string]string{"QUIVER_RELEASE_ASSET_URL": "https://example.test/OLD.AppImage"},
	})
	require.NoError(t, err)

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		domain.Target{},
		domain.OSLinuxAMD64,
		testGetArrow(arrow),
		axRuntime,
		nil,
		nil,
		map[string]string{"QUIVER_RELEASE_ASSET_URL": "https://example.test/NEW.AppImage"},
		updateSteps,
	)

	require.NoError(t, err)
	assert.Equal(t, "https://example.test/NEW.AppImage", vars["QUIVER_RELEASE_ASSET_URL"])
}

// TestResolveVariables_DefaultedVar_StillCarriedForward is the other side of
// the same rule, and the reason it is scoped to no-default variables rather
// than removing layer 5 wholesale. A variable the author gave a fallback to
// is one the author CAN guess, so remembering the last answer refines a
// fallback instead of standing in for an answer nobody gave. The settings a
// user chose at install time still survive to the next run.
func TestResolveVariables_DefaultedVar_StillCarriedForward(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{
		Namespace: ns,
		Variables: []domain.Variable{{Name: "PORT", Default: "25565"}},
	}
	axRuntime := newTestAsynxRuntimeForVars(t)

	_, err := axRuntime.Send(context.Background(), &setStoredVarsCommand{
		ns:         ns,
		storedVars: map[string]string{"PORT": "25570"},
	})
	require.NoError(t, err)

	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		domain.Target{},
		domain.OSLinuxAMD64,
		testGetArrow(arrow),
		axRuntime,
		nil,
		nil,
		nil,
		stepsUnderTest(arrow),
	)

	require.NoError(t, err)
	assert.Equal(t, "25570", vars["PORT"], "a remembered value must still beat the manifest default")
}

// TestResolveVariables_UndeclaredStoredVar_StillCarriedForward keeps the
// filter narrow: a name the manifest never declared is not a required
// variable, so nothing about it is being asked for on every execution and it
// keeps carrying exactly as before.
func TestResolveVariables_UndeclaredStoredVar_StillCarriedForward(t *testing.T) {
	ns := testNsForVars()
	arrow := &domain.Arrow{
		Namespace: ns,
		Variables: []domain.Variable{{Name: "QUIVER_RELEASE_ASSET_URL", Default: ""}},
	}
	axRuntime := newTestAsynxRuntimeForVars(t)

	_, err := axRuntime.Send(context.Background(), &setStoredVarsCommand{
		ns: ns,
		storedVars: map[string]string{
			"QUIVER_RELEASE_ASSET_URL": "https://example.test/OLD.AppImage",
			"SOMETHING_ELSE":           "kept",
		},
	})
	require.NoError(t, err)

	// A method that expands neither, so nothing is required of this call.
	vars, err := assemblerinternal.ResolveVariables(
		context.Background(),
		ns,
		arrow,
		domain.Target{},
		domain.OSLinuxAMD64,
		testGetArrow(arrow),
		axRuntime,
		nil,
		nil,
		nil,
		[]domainStep.Step{domainStep.NewRunStep("noop", "true", false, "10s", true)},
	)

	require.NoError(t, err)
	assert.Equal(t, "kept", vars["SOMETHING_ELSE"])
	assert.NotContains(t, vars, "QUIVER_RELEASE_ASSET_URL",
		"a required variable must not reappear from the previous execution")
}
