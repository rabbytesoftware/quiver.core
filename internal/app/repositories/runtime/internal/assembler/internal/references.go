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

// ReferencedVariables names every variable the given steps actually expand,
// so an execution is only required to supply the variables its own lifecycle
// reads (see requireReferenced). Every Overrideable variant is scanned, not
// just the one this platform resolves to, since under-requiring lets a
// ${...} silently expand to empty. Timeout fields are excluded: they're
// parsed as durations and never expanded.
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
