package assembler

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	assemblerinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/assembler/internal"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
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
		opts ...AssembleOption,
	) (ResolvedExecution, error)
}

// AssembleOption adjusts a single Assemble call.
type AssembleOption func(*assembleOptions)

type assembleOptions struct {
	targetRef string
}

// WithTargetRef sets ${REF} to the ref an update is moving to, overriding the installed one.
func WithTargetRef(ref string) AssembleOption {
	return func(o *assembleOptions) {
		o.targetRef = ref
	}
}

// ReleaseResolver answers the variables a manifest binds to its release.
type ReleaseResolver = assemblerinternal.ReleaseResolver

// ReleaseAssetFn names the asset of a release that an OS runs.
type ReleaseAssetFn = assemblerinternal.ReleaseAssetFn

// NewReleaseResolver builds a ReleaseResolver over a release lookup.
func NewReleaseResolver(
	asset ReleaseAssetFn,
) ReleaseResolver {
	return assemblerinternal.NewReleaseResolver(asset)
}

// Option configures New.
type Option func(*assemblerService)

// WithReleaseResolver lets variables that declare a release source be filled
// from the release the run is built from.
func WithReleaseResolver(
	releases ReleaseResolver,
) Option {
	return func(a *assemblerService) {
		a.releases = releases
	}
}

type assemblerService struct {
	getArrow    GetArrowFn
	getDepArrow GetArrowFn
	axRuntime   asynx.Asynx[domainRuntime.ArrowRuntime]
	vault       vault.Vault
	netbridge   netbridge.Netbridge
	os          domain.OS
	releases    ReleaseResolver
}

func New(
	getArrow GetArrowFn,
	getDepArrow GetArrowFn,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	v vault.Vault,
	nb netbridge.Netbridge,
	os domain.OS,
	opts ...Option,
) Assembler {
	a := &assemblerService{
		getArrow:    getArrow,
		getDepArrow: getDepArrow,
		axRuntime:   axRuntime,
		vault:       v,
		netbridge:   nb,
		os:          os,
	}
	for _, apply := range opts {
		apply(a)
	}
	return a
}

func (a *assemblerService) Assemble(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	userVars map[string]string,
	opts ...AssembleOption,
) (ResolvedExecution, error) {
	var o assembleOptions
	for _, apply := range opts {
		apply(&o)
	}

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
		a.getDepArrow,
		a.axRuntime,
		a.vault,
		a.netbridge,
		userVars,
		// The steps this method is about to run: only the variables THEY
		// expand are required of the caller.
		steps,
		assemblerinternal.WithReleases(a.releases, releaseOf(ns, arrow, o.targetRef)),
	)
	if err != nil {
		return ResolvedExecution{}, err
	}
	if o.targetRef != "" {
		vars[domain.VarRef] = o.targetRef
	}

	var workDir string
	if a.vault != nil {
		workDir, err = a.vault.WorkDir(ctx, ns)
		if err != nil {
			return ResolvedExecution{}, assemblerinternal.WorkDirError(ns, err)
		}
	}

	return ResolvedExecution{
		Steps:       steps,
		Variables:   vars,
		AvailableIn: availableIn,
		WorkDir:     workDir,
	}, nil
}

// releaseOf names the release a run is built from: the target of an update,
// else what the row has installed, else what its selector names.
func releaseOf(
	ns domain.Namespace,
	arrow *domain.Arrow,
	targetRef string,
) domain.Namespace {
	if targetRef != "" {
		return ns.WithRef(targetRef)
	}
	return ns.WithRef(arrow.Resolved.RefOr(ns.Ref()))
}
