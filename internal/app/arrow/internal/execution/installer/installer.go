package installer

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution/runner"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

// Installer owns Install and Uninstall.
type Installer interface {
	Install(
		ctx context.Context,
		ns domain.Namespace,
		userVars map[string]string,
	) error
	Uninstall(
		ctx context.Context,
		ns domain.Namespace,
		userVars map[string]string,
	) error
}

type installerService struct {
	axArrow   asynx.Asynx[domain.Arrow]
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	vault     vault.Vault
	runner    runner.Runner
}

func New(
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	v vault.Vault,
	r runner.Runner,
) (Installer, error) {
	return &installerService{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		vault:     v,
		runner:    r,
	}, nil
}

func (inst *installerService) Install(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	arrow, err := inst.axArrow.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return err
	}
	if errors.Is(err, asynxModels.ErrNotFound) || arrow.Namespace == "" {
		return fmt.Errorf("install: %w", apperrors.ErrNotFound)
	}

	rt, err := inst.axRuntime.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return err
	}
	if rt.Ref != "" && rt.State != domain.ArrowStateAbsent {
		return fmt.Errorf("install: %w", apperrors.ErrStateViolation)
	}

	// Ensure the vault entry exists before execution begins so WORKDIR and
	// INSTALL_PATH are available to all steps. The vault manifest is sourced
	// from the existing vault entry (fetched at add-time).
	vaultEntry, _, vaultErr := inst.vault.GetArrow(ctx, ns)
	if vaultErr != nil {
		return fmt.Errorf("install: vault entry missing: %w", vaultErr)
	}
	if _, err := inst.vault.PutArrow(ctx, ns, vaultEntry.Manifest); err != nil {
		return fmt.Errorf("install: %w", err)
	}

	if err := inst.runner.BeginExecution(ctx, ns, domain.Namespace(""), domain.MethodInstall, userVars); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	return nil
}

func (inst *installerService) Uninstall(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	rt, err := inst.axRuntime.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return err
	}
	if errors.Is(err, asynxModels.ErrNotFound) || rt.Ref == "" || rt.State != domain.ArrowStateReady {
		return fmt.Errorf("uninstall: %w", apperrors.ErrStateViolation)
	}

	if err := inst.runner.BeginExecution(ctx, ns, domain.Namespace(""), domain.MethodUninstall, userVars); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	return nil
}
