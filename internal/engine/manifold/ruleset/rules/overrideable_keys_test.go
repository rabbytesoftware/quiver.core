package rules

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

func TestOverrideableKeysRule_ValidExportSlashKey(t *testing.T) {
	rule := OverrideableKeysRule{}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Exports: map[string]step.Overrideable[string]{
				"BIN": {OSArch: map[string]string{"linux/amd64": "/usr/bin/foo"}},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for valid slash key, got: %v", errs)
	}
}

func TestOverrideableKeysRule_ValidExportWildcardKey(t *testing.T) {
	rule := OverrideableKeysRule{}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Exports: map[string]step.Overrideable[string]{
				"BIN": {OSArch: map[string]string{"*": "/usr/bin/foo"}},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for wildcard key, got: %v", errs)
	}
}

func TestOverrideableKeysRule_InvalidExportBareKey(t *testing.T) {
	rule := OverrideableKeysRule{}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Exports: map[string]step.Overrideable[string]{
				"BIN": {OSArch: map[string]string{"linux": "/usr/bin/foo"}},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for bare key, got: %v", errs)
	}
	if errs[0].Rule != "invalid_overrideable_key" {
		t.Errorf("expected rule invalid_overrideable_key, got %q", errs[0].Rule)
	}
}

func TestOverrideableKeysRule_InvalidRunStepCommandBareKey(t *testing.T) {
	rule := OverrideableKeysRule{}
	run := step.RunStep{
		Command: step.Overrideable[string]{OSArch: map[string]string{"windows": "echo hi"}},
	}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{run},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for bare RunStep command key, got: %v", errs)
	}
	if errs[0].Rule != "invalid_overrideable_key" {
		t.Errorf("expected rule invalid_overrideable_key, got %q", errs[0].Rule)
	}
}

func TestOverrideableKeysRule_ValidRunStepSlashKey(t *testing.T) {
	rule := OverrideableKeysRule{}
	run := step.RunStep{
		Command: step.Overrideable[string]{OSArch: map[string]string{"windows/amd64": "echo hi"}},
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
		t.Fatalf("expected no errors for valid slash key in RunStep, got: %v", errs)
	}
}

func TestOverrideableKeysRule_InvalidFetchStepBareKey(t *testing.T) {
	rule := OverrideableKeysRule{}
	fetch := step.FetchStep{
		URL: step.Overrideable[string]{OSArch: map[string]string{"darwin": "https://example.com"}},
	}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{fetch},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for bare FetchStep url key, got: %v", errs)
	}
}

func TestOverrideableKeysRule_InvalidSignalStepBareSignalKey(t *testing.T) {
	rule := OverrideableKeysRule{}
	signal := step.SignalStep{
		Signal: step.Overrideable[step.SignalKind]{OSArch: map[string]step.SignalKind{"linux": step.SignalKindGraceful}},
	}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{signal},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for bare SignalStep signal key, got: %v", errs)
	}
	if errs[0].Rule != "invalid_overrideable_key" {
		t.Errorf("expected rule invalid_overrideable_key, got %q", errs[0].Rule)
	}
}

func TestOverrideableKeysRule_InvalidRunStepElevatedBareKey(t *testing.T) {
	rule := OverrideableKeysRule{}
	run := step.RunStep{
		Command:  step.Overrideable[string]{Default: "echo ok"},
		Elevated: step.Overrideable[bool]{OSArch: map[string]bool{"linux": true}}, // bare key, invalid
	}
	precompiled := map[string]models.PrecompiledTarget{
		"t": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{run},
			},
		},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for bare RunStep elevated key, got: %v", errs)
	}
	if errs[0].Rule != "invalid_overrideable_key" {
		t.Errorf("expected rule invalid_overrideable_key, got %q", errs[0].Rule)
	}
}

func TestOverrideableKeysRule_ValidRunStepElevatedSlashKey(t *testing.T) {
	rule := OverrideableKeysRule{}
	run := step.RunStep{
		Command:  step.Overrideable[string]{Default: "echo ok"},
		Elevated: step.Overrideable[bool]{OSArch: map[string]bool{"linux/amd64": true}},
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
		t.Fatalf("expected no errors for valid slash elevated key, got: %v", errs)
	}
}

func TestOverrideableKeysRule_EmptyMap(t *testing.T) {
	rule := OverrideableKeysRule{}
	errs := rule.Validate(&domain.Arrow{}, map[string]models.PrecompiledTarget{})
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty map, got: %v", errs)
	}
}
