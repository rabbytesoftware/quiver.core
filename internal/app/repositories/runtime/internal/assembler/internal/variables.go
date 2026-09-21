package assemblerinternal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

// GetArrowFn fetches the current state of an arrow aggregate by namespace.
type GetArrowFn func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)

// ResolveVariables builds the variable map for an execution using 6 priority layers:
// built-ins -> dep built-ins + named exports -> version defaults -> netbridge ports -> stored vars -> user vars.
//
// steps are the ones this execution is about to run, and they are what decides
// which declared variables are REQUIRED -- see requireReferenced. Passing nil
// asks for nothing to be required, which is what a caller with no step list
// wants.
//
// Layer 5 does NOT carry a declared-without-default variable forward; see
// carryForward for why a remembered answer must not stand in for an answer
// this execution was supposed to be given.
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
) (map[string]string, error) {
	vars := make(map[string]string)

	// Layer 1: built-ins
	if v != nil {
		if workdir, err := v.WorkDir(ctx, ns); err == nil {
			vars[domain.VarInstallPath] = workdir
			vars[domain.VarWorkdir] = workdir
		}
	}
	vars[domain.VarArrowNamespace] = ns.String()
	vars[domain.VarPlatform] = os.String()
	vars[domain.VarRef] = ns.Ref()

	// Layer 2: dep built-ins and named exports
	for _, edge := range append(target.Tools, target.Services...) {
		depNs := edge.Namespace.BareNamespace()

		depArrow, err := getArrow(ctx, edge.Namespace)
		if err != nil {
			if !errors.Is(err, asynxModels.ErrNotFound) {
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
			if workdir, err := v.WorkDir(ctx, edge.Namespace); err == nil {
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

	// Layer 6: user vars (highest priority, built-ins excepted)
	applyUserVars(vars, userVars)

	if err := requireReferenced(arrow, steps, vars); err != nil {
		return nil, err
	}

	return vars, nil
}

// carryForward filters a previous execution's variables down to the ones this
// execution may inherit.
//
// A VARIABLE THE MANIFEST DECLARES WITHOUT A DEFAULT IS NOT INHERITED. Such a
// declaration is the arrow author saying "I cannot guess this, ask the
// caller" -- which is exactly what requireReferenced below enforces, by name,
// for every such variable a step expands. Copying the previous execution's
// answer forward silently satisfies that requirement without anyone having
// been asked, turning "required on every execution" into "required once,
// ever", and it does so with a value nothing has revalidated.
//
// The case that made this concrete: quiver.desktop's update lifecycle fetches
// ${QUIVER_RELEASE_ASSET_URL} and verifies ${QUIVER_RELEASE_CHECKSUM}, both
// declared without defaults precisely because only the caller can resolve
// them against the releases API. With the whole map carried forward, a second
// update that named neither would not fail -- it would re-download and
// re-install the asset from the PREVIOUS update, which is the version the
// user already has, or whatever wrong URL was passed the one time. The loud
// failure (ErrMissingVariable, 422, nothing executed) is strictly better than
// a silent reinstall of the wrong build.
//
// Everything else still carries: the built-ins, netbridge's allocated ports,
// dependency exports, any variable the author gave a default to, and any name
// a caller passed that the manifest never declared. Those are the remembered
// settings this layer exists for, and for a defaulted variable a remembered
// value is a refinement of a fallback rather than a substitute for an answer.
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
	if len(required) == 0 {
		return stored
	}

	carried := make(map[string]string, len(stored))
	for name, value := range stored {
		if _, isRequired := required[name]; isRequired {
			continue
		}
		carried[name] = value
	}
	return carried
}

// requireReferenced refuses an execution that is missing a variable its own
// steps are about to expand.
//
// THE SCOPE IS THE METHOD BEING RUN, not the arrow. This used to demand every
// declared no-default variable on every execution, which read as a safety net
// and behaved as a lock: quiver.desktop's uninstall, whose steps expand
// nothing but a path that has a default, was refused for want of a release
// asset URL it never reads -- and the desktop UI sends no variables on
// uninstall or update at all, so both buttons were unreachable as shipped.
// The same rule reached further than the UI: installOneDep begins a
// dependency's install with nil variables, so installing quiver.desktop
// failed on its `tools:` edge to quiver.core, whose own self-manifest
// declares two no-default variables that its (absent) install lifecycle could
// not possibly read.
//
// A variable a step DOES expand is still required, and still by name: an
// update that cannot name the asset it is fetching must fail here, loudly,
// rather than expand to an empty URL and fetch nothing.
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
		if _, resolved := vars[declared.Name]; !resolved {
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
