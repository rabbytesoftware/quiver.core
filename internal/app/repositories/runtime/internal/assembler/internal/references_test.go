package assemblerinternal_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	assemblerinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/assembler/internal"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func names(refs map[string]struct{}) []string {
	out := make([]string, 0, len(refs))
	for name := range refs {
		out = append(out, name)
	}
	return out
}

func TestReferencedVariables_RunStepCommand(t *testing.T) {
	steps := []domainStep.Step{
		domainStep.NewRunStep("place it", `mv ./x "${TARGET_PATH}"`, false, "1m", true),
	}

	assert.ElementsMatch(t, []string{"TARGET_PATH"},
		names(assemblerinternal.ReferencedVariables(steps)))
}

func TestReferencedVariables_FetchStepCoversChecksumToo(t *testing.T) {
	steps := []domainStep.Step{
		domainStep.NewFetchStep("get it", "${ASSET_URL}", "${WORKDIR}/x", "${ASSET_SUM}", "5m", true),
	}

	// Checksum is the one an earlier draft of this could have dropped: the
	// download handler refuses a ${...} that resolved to empty rather than
	// skipping verification, so leaving it unrequired turns a missing value
	// into a failed fetch instead of a clear refusal.
	assert.ElementsMatch(t, []string{"ASSET_URL", "WORKDIR", "ASSET_SUM"},
		names(assemblerinternal.ReferencedVariables(steps)))
}

func TestReferencedVariables_EveryPlatformVariantIsScanned(t *testing.T) {
	step := domainStep.NewRunStep("per-os", "", false, "1m", true)
	step.Command = domainStep.Overrideable[string]{
		Default: "${LINUX_ONLY}",
		OSArch:  map[string]string{"windows/amd64": "${WINDOWS_ONLY}"},
	}

	// A superset, deliberately: reading one variant and guessing wrong would
	// under-require, and an under-required variable expands to empty and
	// silently changes what a step does.
	assert.ElementsMatch(t, []string{"LINUX_ONLY", "WINDOWS_ONLY"},
		names(assemblerinternal.ReferencedVariables([]domainStep.Step{step})))
}

func TestReferencedVariables_StepsWithNothingToExpand(t *testing.T) {
	steps := []domainStep.Step{
		domainStep.NewDependenciesStep("resolve deps"),
		domainStep.NewSignalStep("stop it", domainStep.SignalKindGraceful, "30s", false),
	}

	assert.Empty(t, assemblerinternal.ReferencedVariables(steps))
}

func TestReferencedVariables_NoSteps(t *testing.T) {
	assert.Empty(t, assemblerinternal.ReferencedVariables(nil))
}
