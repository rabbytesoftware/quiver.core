package rules

import (
	"context"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

// AllPrecompile returns all rules that run before OS compilation.
func AllPrecompile() []PrecompileRule {
	return []PrecompileRule{
		VariablesRule{},
		NetbridgeRule{},
		BaseIntegrityRule{},
		OverrideableKeysRule{},
		OverrideableCoverageRule{},
	}
}

// AllCompiled returns all rules that run after OS-specific compilation.
func AllCompiled() []CompiledRule {
	return []CompiledRule{
		ToolsServicesRule{},
		ExportStaticRule{},
		VariableRefsRule{},
		ServicePackageRule{},
		LifecyclePairsRule{},
		TimeoutFormatRule{},
		MethodStatesRule{},
	}
}

// RunPrecompile executes all precompile rules concurrently and collects all errors.
func RunPrecompile(
	ctx context.Context,
	manifest *domain.Arrow,
	precompiled map[string]models.PrecompiledTarget,
) error {
	var (
		mu      sync.Mutex
		allErrs aerrors.RuleErrors
	)

	var wg sync.WaitGroup
	for _, r := range AllPrecompile() {
		wg.Add(1)
		go func(r PrecompileRule) {
			defer wg.Done()
			errs := r.Validate(manifest, precompiled)
			if len(errs) == 0 {
				return
			}
			mu.Lock()
			allErrs = append(allErrs, errs...)
			mu.Unlock()
		}(r)
	}
	wg.Wait()

	if len(allErrs) == 0 {
		return nil
	}
	return allErrs
}

// RunCompiled executes all compiled rules concurrently and collects all errors.
func RunCompiled(
	ctx context.Context,
	manifest *domain.Arrow,
) error {
	var (
		mu      sync.Mutex
		allErrs aerrors.RuleErrors
	)

	var wg sync.WaitGroup
	for _, r := range AllCompiled() {
		wg.Add(1)
		go func(r CompiledRule) {
			defer wg.Done()
			errs := r.Validate(manifest)
			if len(errs) == 0 {
				return
			}
			mu.Lock()
			allErrs = append(allErrs, errs...)
			mu.Unlock()
		}(r)
	}
	wg.Wait()

	if len(allErrs) == 0 {
		return nil
	}
	return allErrs
}
