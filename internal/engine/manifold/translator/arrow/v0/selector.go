package v0

import (
	"fmt"
	"path"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
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

	return buildResolvedTarget(t, os)
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
	if child != nil {
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
		Install:      mergeStepList(parent.Install, child.Install),
		Update:       mergeStepList(parent.Update, child.Update),
		Execute:      mergeStepList(parent.Execute, child.Execute),
		Stop:         mergeStepList(parent.Stop, child.Stop),
		Uninstall:    mergeStepList(parent.Uninstall, child.Uninstall),
		Preinstalled: mergeStepList(parent.Preinstalled, child.Preinstalled),
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
func mergeStepList(parent, child step.StepList) step.StepList {
	if child != nil {
		return child
	}
	return parent
}

func buildResolvedTarget(t models.PrecompiledTarget, os domain.OS) (domain.Target, error) {
	exports, err := resolveExports(t.Exports, os)
	if err != nil {
		return domain.Target{}, err
	}

	lifecycle, err := resolveLifecycle(t.Lifecycle, os)
	if err != nil {
		return domain.Target{}, err
	}

	methods, err := resolveMethods(t.Methods, os)
	if err != nil {
		return domain.Target{}, err
	}

	return domain.Target{
		Requirements: t.Requirements,
		Tools:        toDepEdges(t.Tools, domain.ToolDep),
		Services:     toDepEdges(t.Services, domain.ServiceDep),
		Exports:      exports,
		Lifecycle:    lifecycle,
		Methods:      methods,
	}, nil
}

func resolveExports(
	exports map[string]step.Overrideable[string],
	os domain.OS,
) (map[string]string, error) {
	result := make(map[string]string, len(exports))
	for k, v := range exports {
		val, err := resolveOverrideable(v, os)
		if err != nil {
			return nil, fmt.Errorf("export %q: %w", k, err)
		}
		result[k] = val
	}
	return result, nil
}

func resolveLifecycle(
	lc domain.TargetLifecycle,
	os domain.OS,
) (domain.TargetLifecycle, error) {
	var out domain.TargetLifecycle

	for _, l := range []struct {
		name string
		src  step.StepList
		dst  *step.StepList
	}{
		{"install", lc.Install, &out.Install},
		{"update", lc.Update, &out.Update},
		{"execute", lc.Execute, &out.Execute},
		{"stop", lc.Stop, &out.Stop},
		{"uninstall", lc.Uninstall, &out.Uninstall},
		{"preinstalled", lc.Preinstalled, &out.Preinstalled},
	} {
		resolved, err := resolveStepList(l.src, os)
		if err != nil {
			return domain.TargetLifecycle{}, fmt.Errorf("lifecycle %s: %w", l.name, err)
		}
		*l.dst = resolved
	}

	return out, nil
}

func resolveMethods(
	methods map[string]domain.Method,
	os domain.OS,
) (map[string]domain.Method, error) {
	result := make(map[string]domain.Method, len(methods))
	for name, m := range methods {
		steps, err := resolveStepList(m.Steps, os)
		if err != nil {
			return nil, fmt.Errorf("method %q: %w", name, err)
		}
		result[name] = domain.Method{
			AvailableIn: m.AvailableIn,
			Steps:       steps,
		}
	}
	return result, nil
}

func toDepEdges(namespaces []domain.Namespace, depType domain.DepType) []domain.DependencyEdge {
	edges := make([]domain.DependencyEdge, len(namespaces))
	for i, ns := range namespaces {
		edges[i] = domain.DependencyEdge{
			Namespace:  ns,
			Constraint: ns.Ref(),
			Type:       depType,
		}
	}
	return edges
}

func resolveOverrideable[T any](o step.Overrideable[T], os domain.OS) (T, error) {
	bestRank := -1
	bestKey := ""
	tieKey := ""
	var bestVal T

	for key, val := range o.OSArch {
		if !matchesTarget(key, os) {
			continue
		}
		rank := specificity(key)
		if rank > bestRank {
			bestRank = rank
			bestKey = key
			tieKey = ""
			bestVal = val
		} else if rank == bestRank {
			tieKey = key
		}
	}

	if tieKey != "" {
		var zero T
		return zero, &models.AmbiguousTargetError{Key1: bestKey, Key2: tieKey, OS: string(os)}
	}
	if bestRank >= 0 {
		return bestVal, nil
	}
	return o.Default, nil
}

// resolveStepList flattens every step's Overrideable fields down to the single
// value this OS gets, at compile time — the same moment, and through the same
// resolver, that exports are flattened.
//
// It resolves here rather than calling each step's own Resolve because the
// domain's Overrideable.Resolve is an exact map lookup and cannot be anything
// else: glob matching against an OS/ARCH is real logic with a real failure mode
// (two keys of equal specificity are ambiguous, and picking one out of map
// iteration order would be worse than refusing), and domain/ holds pure types
// with no internal imports — it cannot reach models.AmbiguousTargetError, which
// is what exports have always raised for exactly this. Overrideable.Resolve
// stays what it is and stays correct for what still calls it: a value this
// function has already flattened carries only a Default.
func resolveStepList(steps step.StepList, os domain.OS) (step.StepList, error) {
	if steps == nil {
		return nil, nil
	}
	resolved := make(step.StepList, len(steps))
	for i, s := range steps {
		r, err := resolveStep(s, os)
		if err != nil {
			return nil, fmt.Errorf("step %d %q: %w", i, s.Title(), err)
		}
		resolved[i] = r
	}
	return resolved, nil
}

// resolveStep dispatches on the step's concrete type because which Overrideable
// fields a step has is part of that type, and Go has no generic method to hand
// resolveOverrideable through the Step interface with. A step type with no
// Overrideable fields at all — dependencies, today — has nothing to resolve and
// falls through to its own Resolve, which is the identity.
func resolveStep(s step.Step, os domain.OS) (step.Step, error) {
	switch v := s.(type) {
	case step.RunStep:
		return resolveRunStep(v, os)
	case *step.RunStep:
		return resolveRunStep(*v, os)
	case step.FetchStep:
		return resolveFetchStep(v, os)
	case *step.FetchStep:
		return resolveFetchStep(*v, os)
	case step.SignalStep:
		return resolveSignalStep(v, os)
	case *step.SignalStep:
		return resolveSignalStep(*v, os)
	default:
		return s.Resolve(string(os)), nil
	}
}

func resolveRunStep(s step.RunStep, os domain.OS) (step.Step, error) {
	command, err := resolveField(s.Command, os, "command")
	if err != nil {
		return nil, err
	}
	elevated, err := resolveField(s.Elevated, os, "elevated")
	if err != nil {
		return nil, err
	}
	timeout, err := resolveField(s.Timeout, os, "timeout")
	if err != nil {
		return nil, err
	}

	s.Command = command
	s.Elevated = elevated
	s.Timeout = timeout
	return s, nil
}

func resolveFetchStep(s step.FetchStep, os domain.OS) (step.Step, error) {
	url, err := resolveField(s.URL, os, "url")
	if err != nil {
		return nil, err
	}
	to, err := resolveField(s.To, os, "to")
	if err != nil {
		return nil, err
	}
	checksum, err := resolveField(s.Checksum, os, "checksum")
	if err != nil {
		return nil, err
	}
	timeout, err := resolveField(s.Timeout, os, "timeout")
	if err != nil {
		return nil, err
	}

	s.URL = url
	s.To = to
	s.Checksum = checksum
	s.Timeout = timeout
	return s, nil
}

func resolveSignalStep(s step.SignalStep, os domain.OS) (step.Step, error) {
	signal, err := resolveField(s.Signal, os, "signal")
	if err != nil {
		return nil, err
	}
	timeout, err := resolveField(s.Timeout, os, "timeout")
	if err != nil {
		return nil, err
	}

	s.Signal = signal
	s.Timeout = timeout
	return s, nil
}

// resolveField collapses one Overrideable down to the flattened form a resolved
// step carries: the chosen value as the Default and no OSArch map at all, so
// nothing downstream can re-resolve it against a different OS.
func resolveField[T any](
	o step.Overrideable[T],
	os domain.OS,
	field string,
) (step.Overrideable[T], error) {
	val, err := resolveOverrideable(o, os)
	if err != nil {
		return step.Overrideable[T]{}, fmt.Errorf("%s: %w", field, err)
	}
	return step.Overrideable[T]{Default: val}, nil
}
