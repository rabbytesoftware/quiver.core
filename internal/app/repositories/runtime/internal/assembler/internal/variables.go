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
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

// GetArrowFn fetches the current state of an arrow aggregate by namespace.
type GetArrowFn func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)

// ResolveVariables builds the variable map for an execution using 6 priority layers:
// built-ins -> dep built-ins + named exports -> version defaults -> netbridge ports -> stored vars -> user vars.
func ResolveVariables( //nolint:gocyclo,funlen
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
		maps.Copy(vars, runtime.LastReturn.Variables)
	}

	// Layer 6: user vars (highest priority, built-ins excepted)
	applyUserVars(vars, userVars)

	// Validate: variables with no default must be resolved by now.
	for _, v := range arrow.Variables {
		if v.Default == "" {
			if _, ok := vars[v.Name]; !ok {
				return nil, fmt.Errorf("%w: %q", apperrors.ErrMissingVariable, v.Name)
			}
		}
	}

	return vars, nil
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
