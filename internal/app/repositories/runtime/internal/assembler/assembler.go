package assembler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	assemblerinternal "github.com/rabbytesoftware/quiver/internal/app/repositories/runtime/internal/assembler/internal"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

// GetArrowFn is re-exported so runtime.go can use the type without importing assemblerinternal.
type GetArrowFn = assemblerinternal.GetArrowFn

// ResolvedExecution carries everything needed to begin an execution.
type ResolvedExecution struct {
	Steps       []domainStep.Step
	Variables   map[string]string
	AvailableIn []domain.ArrowState
	WorkDir     string
}

// Assembler translates a namespace + method into a concrete execution plan.
type Assembler interface {
	Assemble(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		userVars map[string]string,
	) (ResolvedExecution, error)
}

type assemblerService struct {
	getArrow  GetArrowFn
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	vault     vault.Vault
	netbridge netbridge.Netbridge
	os        domain.OS
}

func New(
	getArrow GetArrowFn,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	v vault.Vault,
	nb netbridge.Netbridge,
	os domain.OS,
) Assembler {
	return &assemblerService{
		getArrow:  getArrow,
		axRuntime: axRuntime,
		vault:     v,
		netbridge: nb,
		os:        os,
	}
}

func (a *assemblerService) Assemble(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	userVars map[string]string,
) (ResolvedExecution, error) {
	arrow, err := a.getArrow(ctx, ns)
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return ResolvedExecution{}, fmt.Errorf(
				"assemble: %w", apperrors.ErrNotFound,
			)
		}

		return ResolvedExecution{}, fmt.Errorf("assemble: %w", err)
	}

	target, err := assemblerinternal.ResolveTarget(arrow, a.os)
	if err != nil {
		return ResolvedExecution{}, err
	}

	steps, availableIn, err := assemblerinternal.StepsForMethod(
		target,
		method,
	)
	if err != nil {
		return ResolvedExecution{}, err
	}

	vars, err := assemblerinternal.ResolveVariables(
		ctx,
		ns,
		arrow,
		target,
		a.os,
		a.getArrow,
		a.axRuntime,
		a.vault,
		a.netbridge,
		userVars,
	)
	if err != nil {
		return ResolvedExecution{}, err
	}

	var workDir string
	if a.vault != nil {
		workDir, err = a.vault.WorkDir(ctx, ns)
		if err != nil {
			slog.WarnContext(ctx, "assembler: workdir unavailable", "ns", ns, "err", err)
			err = nil // non-fatal: execution proceeds without a WorkDir
		}
	}

	return ResolvedExecution{
		Steps:       steps,
		Variables:   vars,
		AvailableIn: availableIn,
		WorkDir:     workDir,
	}, nil
}
