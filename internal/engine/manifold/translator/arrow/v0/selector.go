package v0

import (
	"fmt"
	"path"
	"strings"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

type selector struct{}

func (s *selector) SelectTarget(
	precompiled map[string]models.PrecompiledTarget,
	os domain.OS,
) (domain.Target, error) {
	return SelectTarget(precompiled, os)
}

func SelectTarget(
	precompiled map[string]models.PrecompiledTarget,
	os domain.OS,
) (domain.Target, error) {
	winnerKey, err := findWinner(precompiled, os)
	if err != nil {
		return domain.Target{}, err
	}

	t, err := flattenBaseChain(winnerKey, precompiled)
	if err != nil {
		return domain.Target{}, err
	}

	return buildResolvedTarget(t, os), nil
}

// IsAbstractTarget reports whether key is an abstract target (starts with "_").
func IsAbstractTarget(key string) bool {
	return strings.HasPrefix(key, "_")
}

func findWinner(targets map[string]models.PrecompiledTarget, os domain.OS) (string, error) {
	bestKey := ""
	tieKey := ""
	bestRank := -1

	for key := range targets {
		bestKey, tieKey, bestRank = updateBest(bestKey, tieKey, bestRank, key, os)
	}

	if tieKey != "" {
		return "", &models.AmbiguousTargetError{Key1: bestKey, Key2: tieKey, OS: string(os)}
	}
	if bestKey == "" {
		return "", models.ErrNoTargetForOS
	}
	return bestKey, nil
}

func updateBest(
	bestKey string,
	tieKey string,
	bestRank int,
	key string,
	os domain.OS,
) (string, string, int) {
	if IsAbstractTarget(key) {
		return bestKey, tieKey, bestRank
	}
	if !matchesTarget(key, os) {
		return bestKey, tieKey, bestRank
	}

	rank := specificity(key)
	if rank > bestRank {
		return key, "", rank
	}
	if rank == bestRank {
		return bestKey, key, bestRank
	}
	return bestKey, tieKey, bestRank
}

func matchesTarget(key string, os domain.OS) bool {
	if key == "*" {
		return true
	}
	matched, err := path.Match(key, string(os))
	return err == nil && matched
}

func specificity(key string) int {
	if key == "*" {
		return 1
	}
	if strings.Contains(key, "*") {
		return 2
	}
	return 3
}

func flattenBaseChain(key string, targets map[string]models.PrecompiledTarget) (models.PrecompiledTarget, error) {
	return walkBaseChain(key, targets, make(map[string]bool))
}

func walkBaseChain(
	key string,
	targets map[string]models.PrecompiledTarget,
	visited map[string]bool,
) (models.PrecompiledTarget, error) {
	if visited[key] {
		return models.PrecompiledTarget{}, fmt.Errorf("base: cycle detected involving %q", key)
	}
	t, ok := targets[key]
	if !ok {
		return models.PrecompiledTarget{}, fmt.Errorf("base: target %q not found", key)
	}
	if t.Base == "" {
		return t, nil
	}

	visited[key] = true
	parent, err := walkBaseChain(t.Base, targets, visited)
	if err != nil {
		return models.PrecompiledTarget{}, err
	}
	return mergeTargets(parent, t), nil
}

func mergeTargets(parent, child models.PrecompiledTarget) models.PrecompiledTarget {
	return models.PrecompiledTarget{
		Base:         "",
		Requirements: mergeRequirements(parent.Requirements, child.Requirements),
		Tools:        mergeNamespaces(parent.Tools, child.Tools),
		Services:     mergeNamespaces(parent.Services, child.Services),
		Exports:      mergeExports(parent.Exports, child.Exports),
		Lifecycle:    mergeLifecycle(parent.Lifecycle, child.Lifecycle),
		Methods:      mergeMethods(parent.Methods, child.Methods),
	}
}

func mergeRequirements(parent, child domain.Requirement) domain.Requirement {
	r := parent
	if child.CpuCores != 0 {
		r.CpuCores = child.CpuCores
	}
	if child.MemoryGB != 0 {
		r.MemoryGB = child.MemoryGB
	}
	if child.DiskGB != 0 {
		r.DiskGB = child.DiskGB
	}
	return r
}

func mergeNamespaces(parent, child []domain.Namespace) []domain.Namespace {
	if len(child) > 0 {
		return child
	}
	return parent
}

func mergeExports(
	parent map[string]step.Overrideable[string],
	child map[string]step.Overrideable[string],
) map[string]step.Overrideable[string] {
	if len(parent) == 0 && len(child) == 0 {
		return nil
	}
	result := make(map[string]step.Overrideable[string], len(parent)+len(child))
	for k, v := range parent {
		result[k] = v
	}
	for k, v := range child {
		result[k] = v
	}
	return result
}

func mergeLifecycle(parent, child domain.TargetLifecycle) domain.TargetLifecycle {
	return domain.TargetLifecycle{
		Install:   mergeStepList(parent.Install, child.Install),
		Update:    mergeStepList(parent.Update, child.Update),
		Execute:   mergeStepList(parent.Execute, child.Execute),
		Stop:      mergeStepList(parent.Stop, child.Stop),
		Uninstall: mergeStepList(parent.Uninstall, child.Uninstall),
	}
}

func mergeMethods(
	parent map[string]domain.Method,
	child map[string]domain.Method,
) map[string]domain.Method {
	if len(parent) == 0 && len(child) == 0 {
		return nil
	}
	result := make(map[string]domain.Method, len(parent)+len(child))
	for k, v := range parent {
		result[k] = v
	}
	for k, v := range child {
		result[k] = v
	}
	return result
}

// mergeStepList merges parent and child step lists.
// nil means "not declared"; non-nil (even empty) means "declared as []".
// child nil → inherit parent; child non-nil (even empty) → child wins.
func mergeStepList(parent step.StepList, child step.StepList) step.StepList {
	if child != nil {
		return child
	}
	return parent
}

func buildResolvedTarget(t models.PrecompiledTarget, os domain.OS) domain.Target {
	exports := make(map[string]string, len(t.Exports))
	for k, v := range t.Exports {
		exports[k] = resolveOverrideable(v, os)
	}

	methods := make(map[string]domain.Method, len(t.Methods))
	for name, m := range t.Methods {
		methods[name] = domain.Method{
			AvailableIn: m.AvailableIn,
			Steps:       resolveStepList(m.Steps, os),
		}
	}

	return domain.Target{
		Requirements: t.Requirements,
		Tools:        t.Tools,
		Services:     t.Services,
		Exports:      exports,
		Lifecycle: domain.TargetLifecycle{
			Install:   resolveStepList(t.Lifecycle.Install, os),
			Update:    resolveStepList(t.Lifecycle.Update, os),
			Execute:   resolveStepList(t.Lifecycle.Execute, os),
			Stop:      resolveStepList(t.Lifecycle.Stop, os),
			Uninstall: resolveStepList(t.Lifecycle.Uninstall, os),
		},
		Methods: methods,
	}
}

func resolveOverrideable[T any](o step.Overrideable[T], os domain.OS) T {
	bestRank := -1
	var bestVal T

	for key, val := range o.OSArch {
		if !matchesTarget(key, os) {
			continue
		}
		rank := specificity(key)
		if rank > bestRank {
			bestRank = rank
			bestVal = val
		}
	}

	if bestRank >= 0 {
		return bestVal
	}
	return o.Default
}

func resolveStepList(steps step.StepList, os domain.OS) step.StepList {
	if steps == nil {
		return nil
	}
	resolved := make(step.StepList, len(steps))
	for i, s := range steps {
		resolved[i] = s.Resolve(string(os))
	}
	return resolved
}
