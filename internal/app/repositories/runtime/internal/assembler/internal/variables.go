package assemblerinternal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/char2cs/asynx"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

// GetArrowFn fetches the current state of an arrow aggregate by namespace.
type GetArrowFn func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)

// Option adjusts a single ResolveVariables call.
type Option func(*resolveOptions)

type resolveOptions struct {
	releases ReleaseResolver
	release  domain.Namespace
}

// WithReleases lets variables that declare a release source take their value
// from release, the namespace at the ref the run is built from.
func WithReleases(
	releases ReleaseResolver,
	release domain.Namespace,
) Option {
	return func(o *resolveOptions) {
		o.releases = releases
		o.release = release
	}
}

// ResolveVariables builds the variable map for an execution using 7 priority
// layers: built-ins -> dep built-ins + named exports -> version defaults ->
// netbridge ports -> stored vars -> release-bound vars -> user vars. steps
// decides which declared variables are required, by name, for this
// execution; see requireReferenced.
func ResolveVariables( //nolint:gocyclo
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
	target domain.Target,
	os domain.OS,
	getArrow GetArrowFn,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	v vault.Vault,
	nb netbridge.Netbridge,
	userVars map[string]string,
	steps []domainStep.Step,
	opts ...Option,
) (map[string]string, error) {
	var o resolveOptions
	for _, apply := range opts {
		apply(&o)
	}

	// Layer 1: built-ins
	vars, err := builtIns(ctx, ns, arrow, os, v)
	if err != nil {
		return nil, err
	}

	// Layer 2: dep built-ins and named exports
	for _, edge := range append(target.Tools, target.Services...) {
		depNs := edge.Namespace.BareNamespace()

		depArrow, err := getArrow(ctx, edge.Namespace)
		if err != nil {
			if !errors.Is(err, apperrors.ErrNotFound) {
				slog.WarnContext(
					ctx,
					"resolveVariables: unexpected error fetching dep",
					"dep",
					depNs,
					"err",
					err,
				)
			}
			continue
		}

		depTarget, ok := depArrow.Targets[os]
		if !ok {
			continue
		}

		// INSTALL_PATH from vault
		if v != nil {
			if workdir, err := v.WorkDir(ctx, depArrow.Namespace); err == nil {
				vars[depNs.String()+".INSTALL_PATH"] = workdir
			}
		}

		// Named exports — anchor relative paths to dep's INSTALL_PATH
		installPath := vars[depNs.String()+".INSTALL_PATH"]
		for exportName, exportValue := range depTarget.Exports {
			resolved := exportValue
			if strings.HasPrefix(exportValue, "./") && installPath != "" {
				resolved = filepath.Join(installPath, exportValue)
			}
			vars[depNs.String()+"."+exportName] = resolved
		}
	}

	// Layer 3: arrow defaults
	for _, v := range arrow.Variables {
		if v.Default != "" {
			vars[v.Name] = v.Default
		}
	}

	// Layer 4: netbridge ports
	if nb != nil { //nolint:nestif
		for _, port := range arrow.Netbridge {
			allocated, err := nb.Allocate(
				ctx,
				ns.String(),
				port.Protocol,
				port.Default,
			)
			if err != nil {
				if port.Required {
					return nil, err
				}
				continue
			}

			vars[port.Name] = strconv.Itoa(allocated)
		}
	}

	// Layer 5: stored vars from last return
	runtime, err := axRuntime.Get(ctx, ns.String())
	if err == nil && runtime.LastReturn != nil {
		maps.Copy(vars, carryForward(arrow, runtime.LastReturn.Variables))
	}

	// Layer 6: variables bound to the release the run is built from
	if err := applyReleaseBound(ctx, o, arrow, os, steps, userVars, vars); err != nil {
		return nil, err
	}

	// Layer 7: user vars (highest priority, built-ins excepted)
	applyUserVars(vars, userVars)

	if err := requireReferenced(arrow, steps, vars); err != nil {
		return nil, err
	}

	return vars, nil
}

// applyReleaseBound fills the variables that declare a release source and that
// this run both expands and was not given by its caller. The lookup happens
// only when something is pending, so a method that expands none of them, or a
// caller that supplied them all, never reaches the host.
func applyReleaseBound(
	ctx context.Context,
	o resolveOptions,
	arrow *domain.Arrow,
	os domain.OS,
	steps []domainStep.Step,
	userVars map[string]string,
	vars map[string]string,
) error {
	if o.releases == nil {
		return nil
	}

	referenced := ReferencedVariables(steps)
	var pending []domain.Variable
	var sources, names []string
	for _, declared := range arrow.Variables {
		if declared.From == "" || userVars[declared.Name] != "" {
			continue
		}
		if _, used := referenced[declared.Name]; !used {
			continue
		}
		pending = append(pending, declared)
		names = append(names, declared.Name)
		if !slices.Contains(sources, declared.From) {
			sources = append(sources, declared.From)
		}
	}
	if len(pending) == 0 {
		return nil
	}

	resolved, err := o.releases.Resolve(ctx, o.release, os, sources)
	if err != nil {
		return fmt.Errorf("resolve %s from release %s: %w", strings.Join(names, ", "), o.release, err)
	}
	for _, declared := range pending {
		if value := resolved[declared.From]; value != "" {
			vars[declared.Name] = value
		}
	}
	return nil
}

// builtIns computes the variables every run gets: its workdir, identity,
// platform and ${REF}.
func builtIns(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
	os domain.OS,
	v vault.Vault,
) (map[string]string, error) {
	vars := make(map[string]string)
	if v != nil {
		workdir, err := v.WorkDir(ctx, ns)
		if err != nil {
			return nil, WorkDirError(ns, err)
		}
		vars[domain.VarInstallPath] = workdir
		vars[domain.VarWorkdir] = workdir
	}
	vars[domain.VarArrowNamespace] = ns.String()
	vars[domain.VarPlatform] = os.String()
	vars[domain.VarRef] = arrow.Resolved.RefOr(ns.Ref())
	return vars, nil
}

// carryForward filters a previous execution's variables down to the ones
// this execution may inherit: answers, never facts. A built-in (${REF},
// ${WORKDIR}, ...) or a dependency's value (<namespace>.<name>) is computed
// for this run: a row advanced since, or a failed update's target left in the
// previous run, must never lend its ${REF} to another release's steps. A
// variable declared without a default is never inherited either, since a
// remembered answer would silently satisfy the per-execution requirement
// requireReferenced enforces without anyone being asked -- e.g. a second
// update naming neither ${QUIVER_RELEASE_ASSET_URL} nor
// ${QUIVER_RELEASE_CHECKSUM} would re-install the previous, possibly wrong,
// build instead of failing loudly.
func carryForward(
	arrow *domain.Arrow,
	stored map[string]string,
) map[string]string {
	required := make(map[string]struct{}, len(arrow.Variables))
	for _, declared := range arrow.Variables {
		if declared.Default == "" {
			required[declared.Name] = struct{}{}
		}
	}

	carried := make(map[string]string, len(stored))
	for name, value := range stored {
		if _, isRequired := required[name]; isRequired || isComputed(name) {
			continue
		}
		carried[name] = value
	}
	return carried
}

// isComputed reports whether name is a value Quiver derives for each run: a
// built-in, or a dependency's value, scoped by the dependency's namespace.
func isComputed(
	name string,
) bool {
	return domain.IsReservedVariable(name) || strings.Contains(name, "/")
}

// requireReferenced refuses an execution missing a variable its own steps
// are about to expand. The scope is the method being run, not the arrow:
// demanding every declared no-default variable on every execution once made
// quiver.desktop's uninstall and update buttons unreachable, since neither
// sends the release asset URL the arrow declares but that method never reads.
func requireReferenced(
	arrow *domain.Arrow,
	steps []domainStep.Step,
	vars map[string]string,
) error {
	referenced := ReferencedVariables(steps)
	for _, declared := range arrow.Variables {
		if declared.Default != "" {
			continue
		}
		if _, used := referenced[declared.Name]; !used {
			continue
		}
		if vars[declared.Name] == "" {
			return fmt.Errorf("%w: %q", apperrors.ErrMissingVariable, declared.Name)
		}
	}
	return nil
}

// applyUserVars copies the caller's variables over the resolved ones, skipping
// the built-ins. Requests carrying a built-in are already rejected upstream;
// this keeps the computed value authoritative for every other caller too.
func applyUserVars(
	vars map[string]string,
	userVars map[string]string,
) {
	for name, value := range userVars {
		if domain.IsReservedVariable(name) {
			continue
		}
		vars[name] = value
	}
}

// WorkDirError reports a workdir the vault could not give ns. It is fatal:
// steps would otherwise run in the daemon's own working directory. A workdir
// another identity owns (a case-folded directory from an earlier layout) is a
// conflict the user resolves by removing that identity.
func WorkDirError(
	ns domain.Namespace,
	err error,
) error {
	if errors.Is(err, vault.ErrWorkDirCollision) {
		return fmt.Errorf("workdir %s: another identity's workdir occupies its path, remove that identity first: %w: %w", ns, apperrors.ErrAlreadyExists, err)
	}
	return fmt.Errorf("workdir %s: %w", ns, err)
}
