package arrow

import (
	"strings"
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
)

func allOSExportKeys() map[string]string {
	keys := make(map[string]string)
	for _, os := range domain.AllOS() {
		keys[string(os)] = "/some/path"
	}
	return keys
}

func TestOverrideableCoverageRule_NonEmptyDefault(t *testing.T) {
	rule := OverrideableCoverageRule{}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Exports: map[string]step.Overrideable[string]{
				"BIN": {Default: "/usr/bin/foo"},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 0 {
		t.Fatalf("expected no errors when default is set, got: %v", errs)
	}
}

func TestOverrideableCoverageRule_WildcardKey(t *testing.T) {
	rule := OverrideableCoverageRule{}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Exports: map[string]step.Overrideable[string]{
				"BIN": {OSArch: map[string]string{"*": "/usr/bin/foo"}},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 0 {
		t.Fatalf("expected no errors with wildcard key, got: %v", errs)
	}
}

func TestOverrideableCoverageRule_AllOSCovered(t *testing.T) {
	rule := OverrideableCoverageRule{}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Exports: map[string]step.Overrideable[string]{
				"BIN": {OSArch: allOSExportKeys()},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 0 {
		t.Fatalf("expected no errors when all OS covered, got: %v", errs)
	}
}

func TestOverrideableCoverageRule_MissingOSCoverage(t *testing.T) {
	rule := OverrideableCoverageRule{}
	// Omit darwin entries
	partial := map[string]string{
		"linux/amd64":   "/bin/foo",
		"linux/arm64":   "/bin/foo",
		"windows/amd64": "C:\\foo.exe",
		"windows/arm64": "C:\\foo.exe",
	}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Exports: map[string]step.Overrideable[string]{
				"BIN": {OSArch: partial},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing OS coverage, got none")
	}
	if errs[0].Rule != "insufficient_coverage" {
		t.Errorf("expected rule insufficient_coverage, got %q", errs[0].Rule)
	}
	if !strings.Contains(errs[0].Message, "darwin") {
		t.Errorf("expected error message to mention missing OS %q, got: %s", "darwin", errs[0].Message)
	}
}

func TestOverrideableCoverageRule_AbstractTargetSkipped(t *testing.T) {
	rule := OverrideableCoverageRule{}
	// Abstract targets (_-prefixed keys) are skipped even with no coverage.
	precompiled := map[string]models.PrecompiledTarget{
		"_abstract": {
			Base: "",
			Exports: map[string]step.Overrideable[string]{
				"BIN": {},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for abstract target, got: %v", errs)
	}
}

func TestOverrideableCoverageRule_ConcreteWithBaseIsValidated(t *testing.T) {
	rule := OverrideableCoverageRule{}
	// A concrete target that inherits via base: must still be validated for coverage.
	precompiled := map[string]models.PrecompiledTarget{
		"linux/*": {
			Base: "_common",
			Exports: map[string]step.Overrideable[string]{
				"BIN": {}, // no default, no OSArch — should fail
			},
		},
		"_common": {},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) == 0 {
		t.Fatal("expected coverage errors for concrete target with base set, got none")
	}
}

func TestOverrideableCoverageRule_BoolFieldNotChecked(t *testing.T) {
	rule := OverrideableCoverageRule{}
	// RunStep.Elevated is Overrideable[bool]; empty default is valid.
	// Set non-empty defaults for all string fields to avoid unrelated errors.
	run := step.RunStep{
		Command:  step.Overrideable[string]{Default: "echo hi"},
		Timeout:  step.Overrideable[string]{Default: "30s"},
		Elevated: step.Overrideable[bool]{},
	}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{run},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for bool Overrideable with empty default, got: %v", errs)
	}
}

func TestOverrideableCoverageRule_FetchStep_MissingOSCoverage(t *testing.T) {
	rule := OverrideableCoverageRule{}
	// Fetch step URL missing darwin coverage
	fetch := step.FetchStep{
		URL: step.Overrideable[string]{OSArch: map[string]string{
			"linux/amd64":   "https://example.com/linux",
			"linux/arm64":   "https://example.com/linux-arm",
			"windows/amd64": "https://example.com/win",
			"windows/arm64": "https://example.com/win-arm",
		}},
		To:      step.Overrideable[string]{Default: "./file"},
		Timeout: step.Overrideable[string]{Default: "10s"},
	}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{fetch},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing darwin coverage in FetchStep, got none")
	}
}

func TestOverrideableCoverageRule_SignalStep_MissingOSCoverage(t *testing.T) {
	rule := OverrideableCoverageRule{}
	signal := step.SignalStep{
		Signal: step.Overrideable[step.SignalKind]{Default: "graceful"},
		Timeout: step.Overrideable[string]{OSArch: map[string]string{
			"linux/amd64": "5s",
			"linux/arm64": "5s",
			// missing darwin, windows
		}},
	}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Lifecycle: domain.TargetLifecycle{
				Stop: step.StepList{signal},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing OS coverage in SignalStep timeout, got none")
	}
}

func TestOverrideableCoverageRule_MethodSteps_MissingCoverage(t *testing.T) {
	rule := OverrideableCoverageRule{}
	run := step.RunStep{
		Command: step.Overrideable[string]{OSArch: map[string]string{
			"linux/amd64": "echo ok",
			"linux/arm64": "echo ok",
			// missing darwin, windows
		}},
		Timeout: step.Overrideable[string]{Default: "10s"},
	}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
			},
			Methods: map[string]domain.Method{
				"check": {
					AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
					Steps:       step.StepList{run},
				},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing OS coverage in method step, got none")
	}
}

func TestOverrideableCoverageRule_FetchStep_OmittedChecksumIsValid(t *testing.T) {
	rule := OverrideableCoverageRule{}
	fetch := step.FetchStep{
		URL:     step.Overrideable[string]{Default: "https://example.com/file"},
		To:      step.Overrideable[string]{Default: "./file"},
		Timeout: step.Overrideable[string]{Default: "10s"},
	}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{fetch},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 0 {
		t.Fatalf("expected no errors when checksum is omitted, got: %v", errs)
	}
}

func TestOverrideableCoverageRule_FetchStep_PartialChecksumFails(t *testing.T) {
	rule := OverrideableCoverageRule{}
	fetch := step.FetchStep{
		URL:     step.Overrideable[string]{Default: "https://example.com/file"},
		To:      step.Overrideable[string]{Default: "./file"},
		Timeout: step.Overrideable[string]{Default: "10s"},
		Checksum: step.Overrideable[string]{OSArch: map[string]string{
			"linux/amd64": "sha256:abc123",
		}},
	}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{fetch},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) == 0 {
		t.Fatal("expected errors when checksum has partial coverage, got none")
	}
}

func TestOverrideableCoverageRule_RunStep_OmittedTimeoutIsValid(t *testing.T) {
	rule := OverrideableCoverageRule{}
	run := step.RunStep{
		Command: step.Overrideable[string]{Default: "echo hi"},
	}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{run},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 0 {
		t.Fatalf("expected no errors when timeout is omitted, got: %v", errs)
	}
}

func TestOverrideableCoverageRule_EmptyMap(t *testing.T) {
	rule := OverrideableCoverageRule{}
	errs := rule.Validate(&domain.Arrow{}, map[string]models.PrecompiledTarget{})
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty map, got: %v", errs)
	}
}

func TestOverrideableCoverageRule_ExactArchKeyDoesNotCoverSiblingArch(t *testing.T) {
	rule := OverrideableCoverageRule{}
	// linux/amd64 is an exact key — it must NOT satisfy coverage for linux/arm64.
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Exports: map[string]step.Overrideable[string]{
				"BIN": {OSArch: map[string]string{
					"linux/amd64":   "/bin/foo-amd64",
					"darwin/amd64":  "/bin/foo-darwin-amd64",
					"darwin/arm64":  "/bin/foo-darwin-arm64",
					"windows/amd64": "C:\\foo-amd64.exe",
					"windows/arm64": "C:\\foo-arm64.exe",
					// linux/arm64 deliberately omitted
				}},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) == 0 {
		t.Fatal("expected coverage error for linux/arm64, got none")
	}
}

func TestOverrideableCoverageRule_GlobArchCoversFamily(t *testing.T) {
	rule := OverrideableCoverageRule{}
	// linux/* is a glob — it SHOULD cover both linux/amd64 and linux/arm64.
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Exports: map[string]step.Overrideable[string]{
				"BIN": {OSArch: map[string]string{
					"linux/*":       "/bin/foo-linux",
					"darwin/amd64":  "/bin/foo-darwin-amd64",
					"darwin/arm64":  "/bin/foo-darwin-arm64",
					"windows/amd64": "C:\\foo-amd64.exe",
					"windows/arm64": "C:\\foo-arm64.exe",
				}},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 0 {
		t.Fatalf("expected no errors when linux/* glob covers all linux variants, got: %v", errs)
	}
}
