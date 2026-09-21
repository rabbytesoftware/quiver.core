package assemblerinternal

import (
	"regexp"

	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

// varTokenRe matches one ${NAME} reference. Identical in shape to the
// manifold ruleset's own (ruleset/arrow/variable_refs.go), because it has to
// agree with it: that rule decides which references a manifest is allowed to
// contain, and this decides which of them an execution must be handed.
var varTokenRe = regexp.MustCompile(`\$\{([^}]+)\}`)

// ReferencedVariables names every variable the given steps actually expand.
//
// It exists so an execution is only required to supply the variables its own
// lifecycle reads. Without it, ResolveVariables demanded every declared
// no-default variable on EVERY method of an arrow, whatever that method did:
// quiver.desktop's uninstall, which touches nothing but a path with a
// default, was refused for want of a release URL it never looks at, and the
// desktop UI's uninstall and update buttons -- which send no variables at all
// -- were unreachable as shipped. The same rule made quiver.core's own
// self-arrow uninstallable through its own API, and broke it as a DEPENDENCY:
// installOneDep begins a dependency's install with nil variables, so
// installing quiver.desktop failed at its `tools:` edge on quiver.core unless
// something had already walked core's self-arrow to Ready by hand.
//
// EVERY Overrideable VARIANT IS SCANNED, not just the one this platform
// resolves to. A superset can only over-require, which is the behaviour that
// was already there; reading one variant and guessing wrong would under-
// require, and an under-required variable is a ${...} that expands to empty
// and silently changes what a step does.
//
// Timeout fields are deliberately absent: they are parsed as durations and
// never expanded (see the fetch and run step handlers), so a reference in one
// would not be substituted anyway.
func ReferencedVariables(
	steps []domainStep.Step,
) map[string]struct{} {
	referenced := make(map[string]struct{})
	for _, s := range steps {
		collectStepReferences(s, referenced)
	}
	return referenced
}

// collectStepReferences adds one step's references. The switch covers the
// step types that carry an expandable field; DependenciesStep, SignalStep and
// anything else carry none.
func collectStepReferences(
	s domainStep.Step,
	into map[string]struct{},
) {
	switch typed := s.(type) {
	case domainStep.RunStep:
		collectOverrideable(typed.Command, into)
	case domainStep.FetchStep:
		collectOverrideable(typed.URL, into)
		collectOverrideable(typed.To, into)
		// Checksum is expanded exactly like the other two, and the download
		// handler refuses a ${...} that resolved to empty rather than
		// skipping verification -- so a checksum reference that went
		// unrequired here would turn a missing value into a failed fetch.
		collectOverrideable(typed.Checksum, into)
	}
}

func collectOverrideable(
	field domainStep.Overrideable[string],
	into map[string]struct{},
) {
	collectTokens(field.Default, into)
	for _, value := range field.OSArch {
		collectTokens(value, into)
	}
}

func collectTokens(
	value string,
	into map[string]struct{},
) {
	for _, match := range varTokenRe.FindAllStringSubmatch(value, -1) {
		into[match[1]] = struct{}{}
	}
}
