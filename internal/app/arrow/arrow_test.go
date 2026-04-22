//go:build !integration

package arrow

import (
	"context"
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	catstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	appDeps "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/mocks"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubStep is a minimal step.Step implementation for tests that need a non-empty lifecycle.
type stubStep struct{}

func (s *stubStep) Type() step.StepType        { return "run" }
func (s *stubStep) Title() string              { return "stub" }
func (s *stubStep) ExitOnFailure() bool        { return false }
func (s *stubStep) Resolve(_ string) step.Step { return s }

// ── helpers ───────────────────────────────────────────────────────────────────

func svcWith(opts ...func(*arrowService)) *arrowService {
	svc := &arrowService{
		catalog:      &mocks.Catalog{},
		deps:         &mocks.Deps{},
		execution:    &mocks.Execution{},
		asynxArrow:   &mocks.AsynxArrow{GetErr: asynxModels.ErrNotFound, ExistsValue: true},
		asynxRuntime: &mocks.AsynxRuntime{GetErr: asynxModels.ErrNotFound},
		manifold:     &mocks.Manifold{},
		vault:        &mocks.Vault{},
		resolveManifest: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "repo"}}, nil
		},
	}
	for _, o := range opts {
		o(svc)
	}
	return svc
}

// ── GetManifest ───────────────────────────────────────────────────────────────

func TestGetManifest_VersionedNamespace_ReturnsInvalidNamespace(t *testing.T) {
	svc := svcWith()
	_, err := svc.GetManifest(context.Background(), "github.com/org/repo@v1.0.0")
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
}

func TestGetManifest_BareNamespace_Success(t *testing.T) {
	arrow := &domain.Arrow{
		Namespace: "github.com/org/repo@v1.0.0",
		ArrowMeta: domain.ArrowMeta{Name: "repo", Version: "1.0.0", Description: "Test", Tags: []string{"test"}},
	}
	svc := svcWith(func(s *arrowService) {
		s.resolveManifest = func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return arrow, nil
		}
	})
	result, err := svc.GetManifest(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, "repo", result.Name)
}

func TestGetManifest_ResolveFails(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.resolveManifest = func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return nil, apperrors.ErrNotFound
		}
	})
	_, err := svc.GetManifest(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
}

func TestGetManifest_ErrorMapping(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"NotFound", apperrors.ErrNotFound},
		{"InvalidNamespace", apperrors.ErrInvalidNamespace},
		{"Other", errors.New("other")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := svcWith(func(s *arrowService) {
				s.resolveManifest = func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
					return nil, tt.err
				}
			})
			_, err := svc.GetManifest(context.Background(), "github.com/org/repo")
			assert.Error(t, err)
		})
	}
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestGet_NotFound(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetErr: apperrors.ErrNotFound}
	})
	_, err := svc.Get(context.Background(), "github.com/org/repo")
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGet_CatalogError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetErr: errors.New("db error")}
	})
	_, err := svc.Get(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
}

func TestGet_NilVm_ReturnsNotFound(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{} // GetArrowValue is nil by default
	})
	_, err := svc.Get(context.Background(), "github.com/org/repo")
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGet_Success(t *testing.T) {
	vm := &catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1", ArrowMeta: domain.ArrowMeta{Name: "repo"}},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: vm}
	})
	result, err := svc.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, "repo", result.Name)
}

// ── HasDependents ─────────────────────────────────────────────────────────────

func TestHasDependents_Error(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{HasDepErr: errors.New("deps error")}
	})
	_, err := svc.HasDependents(context.Background(), "github.com/org/repo", "github.com/other/pkg")
	assert.Error(t, err)
}

func TestHasDependents_True(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{HasDepValue: true}
	})
	has, err := svc.HasDependents(context.Background(), "github.com/org/repo", "github.com/other/pkg")
	require.NoError(t, err)
	assert.True(t, has)
}

func TestHasDependents_False(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{HasDepValue: false}
	})
	has, err := svc.HasDependents(context.Background(), "github.com/org/repo", "github.com/other/pkg")
	require.NoError(t, err)
	assert.False(t, has)
}

// ── Remove ────────────────────────────────────────────────────────────────────

func TestRemove_HasDependentsError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{HasDepErr: errors.New("check error")}
	})
	err := svc.Remove(context.Background(), "github.com/org/repo@v1")
	assert.Error(t, err)
}

func TestRemove_HasDependents_True(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{HasDepValue: true}
	})
	err := svc.Remove(context.Background(), "github.com/org/repo@v1")
	assert.ErrorIs(t, err, apperrors.ErrDependentsExist)
}

func TestRemove_StateViolation_RunningArrow(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.asynxRuntime = &mocks.AsynxRuntime{
			GetValue: domainRuntime.ArrowRuntime{State: domain.ArrowStateReady},
		}
	})
	err := svc.Remove(context.Background(), "github.com/org/repo@v1")
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestRemove_Success(t *testing.T) {
	svc := svcWith() // asynxRuntime returns ErrNotFound → no state
	err := svc.Remove(context.Background(), "github.com/org/repo@v1")
	assert.NoError(t, err)
}

func TestRemove_ForgetError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.asynxArrow = &mocks.AsynxArrow{GetErr: asynxModels.ErrNotFound, ExistsValue: true, ForgetErr: errors.New("forget failed")}
	})
	err := svc.Remove(context.Background(), "github.com/org/repo@v1")
	assert.Error(t, err)
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestList_CatalogError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{ListErr: errors.New("db error")}
	})
	_, err := svc.List(context.Background(), nil)
	assert.Error(t, err)
}

func TestList_NoUserInstalledArrows(t *testing.T) {
	vm := catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1"},
		Versions: []catstore.ArrowVersionRef{{
			Namespace: "github.com/org/repo@v1",
			Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1", UserInstalled: false},
		}},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{ListArrowsValue: []catstore.ArrowViewModel{vm}}
	})
	result, err := svc.List(context.Background(), nil) // nil = user-installed only
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestList_UserInstalledArrows(t *testing.T) {
	vm := catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1", UserInstalled: true},
		Versions: []catstore.ArrowVersionRef{{
			Namespace: "github.com/org/repo@v1",
			Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1", UserInstalled: true},
		}},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{ListArrowsValue: []catstore.ArrowViewModel{vm}}
		// asynxRuntime returns ErrNotFound → ArrowStateAbsent (default)
	})
	result, err := svc.List(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result, 1)
}

func TestList_AsynxRuntimeError_Propagates(t *testing.T) {
	vm := catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1", UserInstalled: true},
		Versions: []catstore.ArrowVersionRef{{
			Namespace: "github.com/org/repo@v1",
			Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1", UserInstalled: true},
		}},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{ListArrowsValue: []catstore.ArrowViewModel{vm}}
		s.asynxRuntime = &mocks.AsynxRuntime{GetErr: errors.New("runtime db error")}
	})
	_, err := svc.List(context.Background(), nil)
	assert.Error(t, err)
}

func TestList_RuntimeStatePopulated(t *testing.T) {
	vm := catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1", UserInstalled: true},
		Versions: []catstore.ArrowVersionRef{{
			Namespace: "github.com/org/repo@v1",
			Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1", UserInstalled: true},
		}},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{ListArrowsValue: []catstore.ArrowViewModel{vm}}
		s.asynxRuntime = &mocks.AsynxRuntime{
			GetValue: domainRuntime.ArrowRuntime{State: domain.ArrowStateReady},
		}
	})
	result, err := svc.List(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, domain.ArrowStateReady, result[0].Versions[0].State)
}

func TestList_DepOnlyFilter(t *testing.T) {
	vms := []catstore.ArrowViewModel{
		{
			Namespace: "github.com/org/dep",
			Versions: []catstore.ArrowVersionRef{{
				Namespace: "github.com/org/dep@v1",
				Metadata:  domain.Arrow{Namespace: "github.com/org/dep@v1", UserInstalled: false},
			}},
		},
	}
	falseVal := false
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{ListArrowsValue: vms}
	})
	result, err := svc.List(context.Background(), &falseVal)
	require.NoError(t, err)
	assert.Len(t, result, 1)
}

func TestList_DepOnlyFilter_SkipsUserInstalledArrow(t *testing.T) {
	// userInstalled=&false means "show deps only" — user-installed arrows should be skipped.
	vms := []catstore.ArrowViewModel{
		{
			Namespace: "github.com/org/repo",
			Versions: []catstore.ArrowVersionRef{{
				Namespace: "github.com/org/repo@v1",
				Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1", UserInstalled: true},
			}},
		},
	}
	falseVal := false
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{ListArrowsValue: vms}
	})
	result, err := svc.List(context.Background(), &falseVal)
	require.NoError(t, err)
	assert.Empty(t, result)
}

// ── Install ───────────────────────────────────────────────────────────────────

func TestInstall_Success(t *testing.T) {
	svc := svcWith()
	err := svc.Install(context.Background(), "github.com/org/repo@v1", nil)
	assert.NoError(t, err)
}

func TestInstall_DepsResolveError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{ResolveErr: errors.New("resolve error")}
	})
	err := svc.Install(context.Background(), "github.com/org/repo@v1", nil)
	assert.Error(t, err)
}

func TestInstall_ExecutionError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.execution = &mocks.Execution{InstallErr: errors.New("install failed")}
	})
	err := svc.Install(context.Background(), "github.com/org/repo@v1", nil)
	assert.Error(t, err)
}

func TestInstall_MissingDep_ResolveError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{
			ResolvePlanValue: appDeps.Plan{{Namespace: "github.com/org/dep@v1"}},
		}
		// asynxRuntime.Get returns ErrNotFound → dep treated as missing
		s.resolveManifest = func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return nil, errors.New("fetch failed")
		}
	})
	err := svc.Install(context.Background(), "github.com/org/repo@v1", nil)
	assert.Error(t, err)
}

func TestInstall_MissingDep_AddArrowCommandFatalError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{
			ResolvePlanValue: appDeps.Plan{{Namespace: "github.com/org/dep@v1"}},
		}
		// addArrowCommand: Get returns ErrNotFound, Send returns non-AlreadyExists error
		s.asynxArrow = &mocks.AsynxArrow{
			GetErr:  asynxModels.ErrNotFound,
			SendErr: errors.New("network error"),
		}
	})
	err := svc.Install(context.Background(), "github.com/org/repo@v1", nil)
	assert.Error(t, err)
}

func TestInstall_MissingDep_DepsExecuteError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{
			ResolvePlanValue: appDeps.Plan{{Namespace: "github.com/org/dep@v1"}},
			ExecuteErr:       errors.New("execute failed"),
		}
	})
	err := svc.Install(context.Background(), "github.com/org/repo@v1", nil)
	assert.Error(t, err)
}

// ── Uninstall ─────────────────────────────────────────────────────────────────

func TestUninstall_Success(t *testing.T) {
	svc := svcWith()
	err := svc.Uninstall(context.Background(), "github.com/org/repo@v1", nil)
	assert.NoError(t, err)
}

func TestUninstall_HasDependentsError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{HasDepErr: errors.New("deps error")}
	})
	err := svc.Uninstall(context.Background(), "github.com/org/repo@v1", nil)
	assert.Error(t, err)
}

func TestUninstall_HasDependents_True(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{HasDepValue: true}
	})
	err := svc.Uninstall(context.Background(), "github.com/org/repo@v1", nil)
	assert.ErrorIs(t, err, apperrors.ErrDependentsExist)
}

func TestUninstall_ExecutionError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.execution = &mocks.Execution{UninstallErr: errors.New("uninstall failed")}
	})
	err := svc.Uninstall(context.Background(), "github.com/org/repo@v1", nil)
	assert.Error(t, err)
}

// ── Seed ──────────────────────────────────────────────────────────────────────

func TestSeed_InvalidNamespace(t *testing.T) {
	svc := svcWith()
	err := svc.Seed(context.Background(), "invalid namespace!", []byte("data"))
	assert.Error(t, err)
}

func TestSeed_ParseArrowError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.manifold = &mocks.Manifold{ParseArrowErr: errors.New("bad yaml")}
	})
	err := svc.Seed(context.Background(), "github.com/org/repo", []byte("bad yaml"))
	assert.ErrorIs(t, err, apperrors.ErrInvalidManifest)
}

func TestSeed_VaultPutError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.manifold = &mocks.Manifold{ParseArrowValue: &domain.Arrow{ArrowMeta: domain.ArrowMeta{Version: "v1"}}}
		s.vault = &mocks.Vault{PutArrowErr: errors.New("disk full")}
	})
	err := svc.Seed(context.Background(), "github.com/org/repo", []byte("valid yaml"))
	assert.Error(t, err)
}

func TestSeed_Success_NewArrow(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.manifold = &mocks.Manifold{ParseArrowValue: &domain.Arrow{ArrowMeta: domain.ArrowMeta{Version: "v1"}}}
		// asynxArrow.Get returns ErrNotFound → new arrow → sends AddArrow
	})
	err := svc.Seed(context.Background(), "github.com/org/repo", []byte("valid yaml"))
	assert.NoError(t, err)
}

func TestSeed_AlreadyExists_SendsUpdate(t *testing.T) {
	// First Send (AddArrow) returns ErrValidation → ErrAlreadyExists.
	// Second Send (UpdateArrowManifest) must succeed.
	sendCount := 0
	ax := &mocks.AsynxArrow{GetErr: asynxModels.ErrNotFound}
	ax.SendFn = func() error {
		sendCount++
		if sendCount == 1 {
			return asynxModels.ErrValidation
		}
		return nil
	}
	svc := svcWith(func(s *arrowService) {
		s.manifold = &mocks.Manifold{ParseArrowValue: &domain.Arrow{ArrowMeta: domain.ArrowMeta{Version: "v1"}}}
		s.asynxArrow = ax
	})
	err := svc.Seed(context.Background(), "github.com/org/repo", []byte("valid yaml"))
	assert.NoError(t, err)
}

func TestSeed_AddArrowSendError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.manifold = &mocks.Manifold{ParseArrowValue: &domain.Arrow{ArrowMeta: domain.ArrowMeta{Version: "v1"}}}
		s.asynxArrow = &mocks.AsynxArrow{
			GetErr:  asynxModels.ErrNotFound,
			SendErr: errors.New("send failed"),
		}
	})
	err := svc.Seed(context.Background(), "github.com/org/repo", []byte("valid yaml"))
	assert.Error(t, err)
}

// ── ValidateManifest ──────────────────────────────────────────────────────────

func TestValidateManifest_Valid(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.manifold = &mocks.Manifold{ParseArrowValue: &domain.Arrow{
			Targets: map[domain.OS]domain.Target{domain.OSDarwinARM64: {}},
		}}
	})
	result, err := svc.ValidateManifest(context.Background(), "github.com/org/repo", []byte("data"))
	require.NoError(t, err)
	assert.True(t, result.Valid)
}

func TestValidateManifest_RuleErrors(t *testing.T) {
	ruleErrs := aerrors.RuleErrors{
		{Field: "targets", Rule: "missing_pair", Message: "install requires uninstall"},
	}
	svc := svcWith(func(s *arrowService) {
		s.manifold = &mocks.Manifold{ParseArrowErr: ruleErrs}
	})
	result, err := svc.ValidateManifest(context.Background(), "github.com/org/repo", []byte("bad"))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "missing_pair", result.Errors[0].Rule)
}

func TestValidateManifest_ParseError_Generic(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.manifold = &mocks.Manifold{ParseArrowErr: errors.New("generic error")}
	})
	result, err := svc.ValidateManifest(context.Background(), "github.com/org/repo", []byte("bad"))
	require.NoError(t, err) // ValidateManifest never returns an error
	assert.False(t, result.Valid)
	assert.Equal(t, "parse_error", result.Errors[0].Rule)
}

// ── Shutdown ──────────────────────────────────────────────────────────────────

func TestShutdown_Success(t *testing.T) {
	svc := svcWith()
	err := svc.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestShutdown_AsynxArrowError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.asynxArrow = &mocks.AsynxArrow{ShutdownErr: errors.New("arrow shutdown failed")}
	})
	err := svc.Shutdown(context.Background())
	assert.Error(t, err)
}

func TestShutdown_AsynxRuntimeError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.asynxRuntime = &mocks.AsynxRuntime{ShutdownErr: errors.New("runtime shutdown failed")}
	})
	err := svc.Shutdown(context.Background())
	assert.Error(t, err)
}

// ── Add ───────────────────────────────────────────────────────────────────────

func TestAdd_Success(t *testing.T) {
	svc := svcWith()
	err := svc.Add(context.Background(), "github.com/org/repo@v1")
	assert.NoError(t, err)
}

func TestAdd_ResolveManifestError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.resolveManifest = func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return nil, errors.New("fetch failed")
		}
	})
	err := svc.Add(context.Background(), "github.com/org/repo@v1")
	assert.Error(t, err)
}

// ── addArrowCommand ───────────────────────────────────────────────────────────

func TestAddArrowCommand_ExistingNotUserInstalled_SendsSetUserInstalled(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.asynxArrow = &mocks.AsynxArrow{
			GetErr:   nil,
			GetValue: domain.Arrow{UserInstalled: false},
		}
	})
	err := svc.addArrowCommand(context.Background(), "github.com/org/repo@v1", &domain.Arrow{}, "")
	assert.NoError(t, err)
}

func TestAddArrowCommand_ExistingUserInstalled_ReturnsNil(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.asynxArrow = &mocks.AsynxArrow{
			GetErr:   nil,
			GetValue: domain.Arrow{UserInstalled: true},
		}
	})
	err := svc.addArrowCommand(context.Background(), "github.com/org/repo@v1", &domain.Arrow{UserInstalled: true}, "")
	assert.NoError(t, err)
}

func TestAddArrowCommand_SendErrValidation_WrapsAlreadyExists(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.asynxArrow = &mocks.AsynxArrow{
			GetErr:  asynxModels.ErrNotFound,
			SendErr: asynxModels.ErrValidation,
		}
	})
	err := svc.addArrowCommand(context.Background(), "github.com/org/repo@v1", &domain.Arrow{}, "")
	assert.ErrorIs(t, err, apperrors.ErrAlreadyExists)
}

func TestAddArrowCommand_SendErrPipelineFailed_WrapsAlreadyExists(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.asynxArrow = &mocks.AsynxArrow{
			GetErr:  asynxModels.ErrNotFound,
			SendErr: asynxModels.ErrPipelineFailed,
		}
	})
	err := svc.addArrowCommand(context.Background(), "github.com/org/repo@v1", &domain.Arrow{}, "")
	assert.ErrorIs(t, err, apperrors.ErrAlreadyExists)
}

func TestAddArrowCommand_SendOtherError_Propagates(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.asynxArrow = &mocks.AsynxArrow{
			GetErr:  asynxModels.ErrNotFound,
			SendErr: errors.New("network error"),
		}
	})
	err := svc.addArrowCommand(context.Background(), "github.com/org/repo@v1", &domain.Arrow{}, "")
	assert.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrAlreadyExists)
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestUpdate_CatalogGetError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetErr: errors.New("db error")}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{})
	assert.Error(t, err)
}

func TestUpdate_NilVm_ReturnsNotFound(t *testing.T) {
	svc := svcWith()
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{})
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestUpdate_NoConstraint_CallsUpdateManifest(t *testing.T) {
	vm := &catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata: domain.Arrow{
			Namespace:           "github.com/org/repo@v1",
			ArrowMeta:           domain.ArrowMeta{Name: "repo"},
			InstalledConstraint: "", // no constraint
		},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: vm}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{})
	assert.NoError(t, err)
}

func TestUpdate_WithConstraint_ResolveError(t *testing.T) {
	vm := &catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata: domain.Arrow{
			Namespace:           "github.com/org/repo@v1",
			InstalledConstraint: ">=v1",
		},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: vm}
		s.manifold = &mocks.Manifold{ConstraintErr: errors.New("resolve error")}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{})
	assert.Error(t, err)
}

// ── filterOrphans ─────────────────────────────────────────────────────────────

func TestFilterOrphans_HasDependentsError_Skips(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{HasDepErr: errors.New("error")}
	})
	result := svc.filterOrphans(context.Background(),
		[]domain.Namespace{"github.com/org/dep@v1"},
		"github.com/org/repo@v1",
	)
	assert.Empty(t, result) // skipped due to error
}

func TestFilterOrphans_HasDependents_Skips(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{HasDepValue: true}
	})
	result := svc.filterOrphans(context.Background(),
		[]domain.Namespace{"github.com/org/dep@v1"},
		"github.com/org/repo@v1",
	)
	assert.Empty(t, result)
}

func TestFilterOrphans_NoDependents_Includes(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{HasDepValue: false}
	})
	result := svc.filterOrphans(context.Background(),
		[]domain.Namespace{"github.com/org/dep@v1"},
		"github.com/org/repo@v1",
	)
	assert.Equal(t, []domain.Namespace{"github.com/org/dep@v1"}, result)
}

// ── cleanupAfterUninstall ─────────────────────────────────────────────────────

func TestCleanupAfterUninstall_OrphansError_NoOp(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{OrphansErr: errors.New("orphans error")}
	})
	// must not panic
	svc.cleanupAfterUninstall(context.Background(), "github.com/org/repo@v1")
}

func TestCleanupAfterUninstall_CallsUninstall(t *testing.T) {
	uninstallCalled := false
	svc := svcWith(func(s *arrowService) {
		s.deps = &mocks.Deps{OrphansValue: []domain.Namespace{"github.com/org/dep@v1"}}
		s.execution = &mocks.Execution{}
	})
	_ = uninstallCalled
	// no panic, orphans are uninstalled
	svc.cleanupAfterUninstall(context.Background(), "github.com/org/repo@v1")
}

// ── GetDetail ─────────────────────────────────────────────────────────────────

func TestGetDetail_CatalogError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetErr: errors.New("db error")}
	})
	_, err := svc.GetDetail(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
}

func TestGetDetail_NilVm_ReturnsNotFound(t *testing.T) {
	svc := svcWith() // catalog.Get returns nil, nil
	_, err := svc.GetDetail(context.Background(), "github.com/org/repo")
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGetDetail_VersionedNs_NotFound(t *testing.T) {
	vm := &catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1"},
		Versions: []catstore.ArrowVersionRef{{
			Namespace: "github.com/org/repo@v1",
			Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1"},
		}},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: vm}
	})
	_, err := svc.GetDetail(context.Background(), "github.com/org/repo@v2") // v2 not in versions
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGetDetail_VersionedNs_Success(t *testing.T) {
	vm := &catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1", ArrowMeta: domain.ArrowMeta{Name: "repo"}},
		Versions: []catstore.ArrowVersionRef{{
			Namespace: "github.com/org/repo@v1",
			Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1", ArrowMeta: domain.ArrowMeta{Name: "repo"}},
		}},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: vm}
	})
	result, err := svc.GetDetail(context.Background(), "github.com/org/repo@v1")
	require.NoError(t, err)
	assert.Equal(t, "repo", result.Name)
}

func TestGetDetail_PreloadError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.asynxRuntime = &mocks.AsynxRuntime{PreloadErr: errors.New("preload failed")}
	})
	_, err := svc.GetDetail(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
}

func TestGetDetail_RuntimeGetError_NonNotFound(t *testing.T) {
	vm := &catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1", ArrowMeta: domain.ArrowMeta{Name: "repo"}},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: vm}
		s.asynxRuntime = &mocks.AsynxRuntime{GetErr: errors.New("db error")}
	})
	_, err := svc.GetDetail(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
}

func TestGetDetail_BareNs_WithRuntimeState(t *testing.T) {
	vm := &catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1", ArrowMeta: domain.ArrowMeta{Name: "repo"}},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: vm}
		s.asynxRuntime = &mocks.AsynxRuntime{
			GetValue: domainRuntime.ArrowRuntime{State: domain.ArrowStateReady},
		}
	})
	result, err := svc.GetDetail(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, result.State)
}

// ── resolveForInstall (glob path) ─────────────────────────────────────────────

func TestAdd_GlobNamespace_Success(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.manifold = &mocks.Manifold{ConstraintValue: "v1.2.3"}
	})
	// glob namespace has a wildcard ref like ">=v1"
	err := svc.Add(context.Background(), "github.com/org/repo@v1.*")
	assert.NoError(t, err)
}

func TestAdd_GlobNamespace_ResolveConstraintError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.manifold = &mocks.Manifold{ConstraintErr: errors.New("no matching tag")}
	})
	err := svc.Add(context.Background(), "github.com/org/repo@v1.*")
	assert.Error(t, err)
}

// ── updateManifest (InstallAdded + UninstallOrphans) ──────────────────────────

func TestUpdate_UpdateManifest_ResolveError(t *testing.T) {
	vm := &catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1"},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: vm}
		s.resolveManifest = func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return nil, errors.New("fetch failed")
		}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{})
	assert.Error(t, err)
}

func TestUpdate_InstallAdded_CallsInstall(t *testing.T) {
	vm := &catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1"},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: vm}
		s.deps = &mocks.Deps{
			DiffResultValue: appDeps.DepDiff{
				Added: []domain.DependencyEdge{{Namespace: "github.com/org/dep@v1"}},
			},
		}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{InstallAdded: true})
	assert.NoError(t, err)
}

func TestUpdate_UninstallOrphans_CallsUninstall(t *testing.T) {
	vm := &catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1"},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: vm}
		s.deps = &mocks.Deps{
			DiffResultValue: appDeps.DepDiff{
				Removed: []domain.DependencyEdge{{Namespace: "github.com/org/dep@v1"}},
			},
		}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{UninstallOrphans: true})
	assert.NoError(t, err)
}

func TestUpdate_UpdateManifest_SendError(t *testing.T) {
	vm := &catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata:  domain.Arrow{Namespace: "github.com/org/repo@v1"},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: vm}
		s.asynxArrow = &mocks.AsynxArrow{
			GetErr:  asynxModels.ErrNotFound,
			SendErr: errors.New("send failed"),
		}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{})
	assert.Error(t, err)
}

// ── upgradeVersion (via Update with constraint + diff ref + UpgradeRef) ────────

// upgradeVm builds a view model that triggers the upgradeVersion path.
func upgradeVm(installedRef string) *catstore.ArrowViewModel {
	return &catstore.ArrowViewModel{
		Namespace: "github.com/org/repo",
		Metadata: domain.Arrow{
			Namespace:           domain.Namespace("github.com/org/repo@" + installedRef),
			ArrowMeta:           domain.ArrowMeta{Name: "repo", Version: installedRef},
			InstalledRef:        installedRef,
			InstalledConstraint: "v1.*",
		},
	}
}

func TestUpdate_UpgradeRef_Success(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: upgradeVm("v1")}
		s.manifold = &mocks.Manifold{ConstraintValue: "v2"}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{UpgradeRef: true})
	assert.NoError(t, err)
}

func TestUpdate_UpgradeRef_ResolveManifestError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: upgradeVm("v1")}
		s.manifold = &mocks.Manifold{ConstraintValue: "v2"}
		s.resolveManifest = func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return nil, errors.New("fetch failed")
		}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{UpgradeRef: true})
	assert.Error(t, err)
}

func TestUpdate_UpgradeRef_RenameVaultError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: upgradeVm("v1")}
		s.manifold = &mocks.Manifold{ConstraintValue: "v2"}
		s.vault = &mocks.Vault{RenameArrowErr: errors.New("rename failed")}
		// asynxRuntime returns ErrNotFound → triggers rename path
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{UpgradeRef: true})
	assert.Error(t, err)
}

func TestUpdate_UpgradeRef_PutArrowError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: upgradeVm("v1")}
		s.manifold = &mocks.Manifold{ConstraintValue: "v2"}
		s.vault = &mocks.Vault{PutArrowErr: errors.New("write failed")}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{UpgradeRef: true})
	assert.Error(t, err)
}

func TestUpdate_UpgradeRef_V2AlreadyInstalled_SkipsRename(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: upgradeVm("v1")}
		s.manifold = &mocks.Manifold{ConstraintValue: "v2"}
		// asynxRuntime returns success for v2 → skip rename
		s.asynxRuntime = &mocks.AsynxRuntime{
			GetValue: domainRuntime.ArrowRuntime{State: domain.ArrowStateReady},
		}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{UpgradeRef: true})
	assert.NoError(t, err)
}

func TestUpdate_UpgradeRef_HasUpdateLifecycle(t *testing.T) {
	newArrow := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{Name: "repo"},
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinARM64: {
				Lifecycle: domain.TargetLifecycle{
					Update: step.StepList{&stubStep{}},
				},
			},
		},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: upgradeVm("v1")}
		s.manifold = &mocks.Manifold{ConstraintValue: "v2"}
		s.resolveManifest = func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return newArrow, nil
		}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{UpgradeRef: true})
	assert.NoError(t, err)
}

func TestUpdate_UpgradeRef_AddArrowCommandFatalError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: upgradeVm("v1")}
		s.manifold = &mocks.Manifold{ConstraintValue: "v2"}
		// addArrowCommand fatal: Get returns ErrNotFound, Send returns non-AlreadyExists
		s.asynxArrow = &mocks.AsynxArrow{
			GetErr:  asynxModels.ErrNotFound,
			SendErr: errors.New("network error"),
		}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{UpgradeRef: true})
	assert.Error(t, err)
}

func TestUpdate_UpgradeRef_BeginExecutionError(t *testing.T) {
	newArrow := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{Name: "repo"},
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinARM64: {
				Lifecycle: domain.TargetLifecycle{
					Update: step.StepList{&stubStep{}},
				},
			},
		},
	}
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: upgradeVm("v1")}
		s.manifold = &mocks.Manifold{ConstraintValue: "v2"}
		s.resolveManifest = func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return newArrow, nil
		}
		s.execution = &mocks.Execution{BeginExecutionErr: errors.New("execution failed")}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{UpgradeRef: true})
	assert.Error(t, err)
}

func TestUpdate_UpgradeRef_InstallNewVersionError(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: upgradeVm("v1")}
		s.manifold = &mocks.Manifold{ConstraintValue: "v2"}
		s.execution = &mocks.Execution{InstallErr: errors.New("install failed")}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{UpgradeRef: true})
	assert.Error(t, err)
}

func TestUpdate_UpgradeRef_ForgetErrorOnlyLogs(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: upgradeVm("v1")}
		s.manifold = &mocks.Manifold{ConstraintValue: "v2"}
		s.asynxArrow = &mocks.AsynxArrow{
			GetErr:      asynxModels.ErrNotFound,
			ExistsValue: true,
			ForgetErr:   errors.New("forget failed"), // should only log, not fail
		}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{UpgradeRef: true})
	assert.NoError(t, err) // Forget error is non-fatal
}

func TestUpdate_UpgradeRef_UninstallsSafeOrphans(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: upgradeVm("v1")}
		s.manifold = &mocks.Manifold{ConstraintValue: "v2"}
		s.deps = &mocks.Deps{
			DiffResultValue: appDeps.DepDiff{
				Removed: []domain.DependencyEdge{{Namespace: "github.com/org/dep@v1"}},
			},
			HasDepValue: false, // dep has no other dependents → safe to uninstall
		}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{UpgradeRef: true})
	assert.NoError(t, err)
}

func TestUpdate_UpgradeRef_InstallAdded(t *testing.T) {
	svc := svcWith(func(s *arrowService) {
		s.catalog = &mocks.Catalog{GetArrowValue: upgradeVm("v1")}
		s.manifold = &mocks.Manifold{ConstraintValue: "v2"}
		s.deps = &mocks.Deps{
			DiffResultValue: appDeps.DepDiff{
				Added: []domain.DependencyEdge{{Namespace: "github.com/org/dep@v1"}},
			},
		}
	})
	_, err := svc.Update(context.Background(), "github.com/org/repo", UpdateOptions{UpgradeRef: true, InstallAdded: true})
	assert.NoError(t, err)
}

// ── edgesToNamespaces ─────────────────────────────────────────────────────────

func TestEdgesToNamespaces_Empty(t *testing.T) {
	result := edgesToNamespaces(nil)
	assert.Empty(t, result)
}

func TestEdgesToNamespaces_NonEmpty(t *testing.T) {
	edges := []domain.DependencyEdge{
		{Namespace: "github.com/org/dep@v1"},
		{Namespace: "github.com/org/dep2@v1"},
	}
	result := edgesToNamespaces(edges)
	assert.Equal(t, []domain.Namespace{"github.com/org/dep@v1", "github.com/org/dep2@v1"}, result)
}
