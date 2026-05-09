package v0_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	v0 "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator/arrow/v0"
)

func makeInstallTarget(cmd string) models.PrecompiledTarget {
	return models.PrecompiledTarget{
		Lifecycle: domain.TargetLifecycle{
			Install: step.StepList{
				step.NewRunStep("install", cmd, false, "", true),
			},
			Uninstall: step.StepList{},
		},
	}
}

func makeTargets(targets map[string]models.PrecompiledTarget) map[string]models.PrecompiledTarget {
	return targets
}

func TestSelectTarget_ExactMatch(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"linux/amd64": makeInstallTarget("apt install foo"),
	})
	_, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSelectTarget_GlobMatch(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"linux/*": makeInstallTarget("apt install foo"),
	})

	_, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("linux/amd64: unexpected error: %v", err)
	}

	_, err = v0.SelectTarget(targets, domain.OSLinuxARM64)
	if err != nil {
		t.Fatalf("linux/arm64: unexpected error: %v", err)
	}

	_, err = v0.SelectTarget(targets, domain.OSDarwinAMD64)
	if !errors.Is(err, models.ErrNoTargetForOS) {
		t.Fatalf("darwin/amd64: expected ErrNoTargetForOS, got %v", err)
	}
}

func TestSelectTarget_CatchAll(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"*": makeInstallTarget("install-all"),
	})
	for _, os := range domain.AllOS() {
		_, err := v0.SelectTarget(targets, os)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", os, err)
		}
	}
}

func TestSelectTarget_ExactBeatsGlob(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"linux/amd64": makeInstallTarget("exact-cmd"),
		"linux/*":     makeInstallTarget("glob-cmd"),
	})
	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := rt.Lifecycle.Install[0].(step.RunStep).Command.Default
	if got != "exact-cmd" {
		t.Fatalf("expected exact-cmd, got %q", got)
	}
}

func TestSelectTarget_GlobBeatsCatchAll(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"linux/*": makeInstallTarget("glob-cmd"),
		"*":       makeInstallTarget("catch-all-cmd"),
	})
	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := rt.Lifecycle.Install[0].(step.RunStep).Command.Default
	if got != "glob-cmd" {
		t.Fatalf("expected glob-cmd, got %q", got)
	}
}

func TestSelectTarget_Tie(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"linux/*": makeInstallTarget("linux-cmd"),
		"*/amd64": makeInstallTarget("amd64-cmd"),
	})
	_, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	var ambig *models.AmbiguousTargetError
	if !errors.As(err, &ambig) {
		t.Fatalf("expected *AmbiguousTargetError, got %v", err)
	}
}

func TestSelectTarget_NoMatch(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"linux/*": makeInstallTarget("linux-cmd"),
	})
	_, err := v0.SelectTarget(targets, domain.OSDarwinAMD64)
	if !errors.Is(err, models.ErrNoTargetForOS) {
		t.Fatalf("expected ErrNoTargetForOS, got %v", err)
	}
}

func TestSelectTarget_AbstractSkipped(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"_abstract": makeInstallTarget("abstract-cmd"),
	})
	_, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if !errors.Is(err, models.ErrNoTargetForOS) {
		t.Fatalf("expected ErrNoTargetForOS, got %v", err)
	}
}

func TestSelectTarget_BaseChainInherited(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"_common": {
			Lifecycle: domain.TargetLifecycle{
				Execute:   step.StepList{step.NewRunStep("execute", "run-it", false, "", true)},
				Stop:      step.StepList{step.NewRunStep("stop", "stop-it", false, "", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "remove-it", false, "", true)},
			},
		},
		"linux/*": {
			Base: "_common",
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{step.NewRunStep("install", "apt install foo", false, "", true)},
			},
		},
	})

	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rt.Lifecycle.Install) == 0 {
		t.Fatal("expected install steps from child")
	}
	if len(rt.Lifecycle.Execute) == 0 {
		t.Fatal("expected execute steps inherited from _common")
	}
	if len(rt.Lifecycle.Stop) == 0 {
		t.Fatal("expected stop steps inherited from _common")
	}
	if len(rt.Lifecycle.Uninstall) == 0 {
		t.Fatal("expected uninstall steps inherited from _common")
	}
}

func TestSelectTarget_ChildOverridesParentInstall(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"_base": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{step.NewRunStep("install", "parent-install", false, "", true)},
			},
		},
		"linux/*": {
			Base: "_base",
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{step.NewRunStep("install", "child-install", false, "", true)},
			},
		},
	})

	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := rt.Lifecycle.Install[0].(step.RunStep).Command.Default
	if got != "child-install" {
		t.Fatalf("expected child-install, got %q", got)
	}
}

func TestSelectTarget_Cycle(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"_a":      {Base: "_b"},
		"_b":      {Base: "_a"},
		"linux/*": {Base: "_a"},
	})
	_, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err == nil {
		t.Fatal("expected error for cycle, got nil")
	}
}

func TestSelectTarget_MissingBase(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"linux/*": {Base: "_nonexistent"},
	})
	_, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err == nil {
		t.Fatal("expected error for missing base, got nil")
	}
}

func TestSelectTarget_AmbiguousExportOSArch_ReturnsError(t *testing.T) {
	// Two OSArch keys with equal specificity (rank 2) both matching linux/amd64.
	// resolveOverrideable must error rather than pick non-deterministically.
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"linux/*": {
			Exports: map[string]step.Overrideable[string]{
				"BIN": {
					OSArch: map[string]string{
						"linux/*": "/bin/linux",
						"*/amd64": "/bin/amd64",
					},
				},
			},
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "", true)},
			},
		},
	})
	_, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	var ambig *models.AmbiguousTargetError
	if !errors.As(err, &ambig) {
		t.Fatalf("expected *AmbiguousTargetError for tied export OSArch keys, got %v", err)
	}
}

func TestAmbiguousTargetError_Error(t *testing.T) {
	err := &models.AmbiguousTargetError{Key1: "linux/*", Key2: "*/amd64", OS: "linux/amd64"}
	msg := err.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
	if !containsAll(msg, "linux/*", "*/amd64", "linux/amd64") {
		t.Errorf("error message %q missing expected content", msg)
	}
}

func TestSelectTarget_MergeRequirements_ChildOverridesParent(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"_base": {
			Requirements: domain.Requirement{
				CpuCores: 2,
				MemoryGB: 4,
				DiskGB:   10,
			},
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "", true)},
			},
		},
		"linux/*": {
			Base: "_base",
			Requirements: domain.Requirement{
				CpuCores: 4,
				MemoryGB: 8,
			},
			Lifecycle: domain.TargetLifecycle{},
		},
	})
	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Requirements.CpuCores != 4 {
		t.Errorf("CpuCores = %d, want 4 (overridden by child)", rt.Requirements.CpuCores)
	}
	if rt.Requirements.MemoryGB != 8 {
		t.Errorf("MemoryGB = %d, want 8 (overridden by child)", rt.Requirements.MemoryGB)
	}
	if rt.Requirements.DiskGB != 10 {
		t.Errorf("DiskGB = %d, want 10 (inherited from parent)", rt.Requirements.DiskGB)
	}
}

func TestSelectTarget_MergeRequirements_ChildPartial(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"_base": {
			Requirements: domain.Requirement{
				CpuCores: 2,
				MemoryGB: 4,
				DiskGB:   10,
			},
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "", true)},
			},
		},
		"linux/*": {
			Base: "_base",
			Requirements: domain.Requirement{
				MemoryGB: 8,
			},
			Lifecycle: domain.TargetLifecycle{},
		},
	})
	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Requirements.CpuCores != 2 {
		t.Errorf("CpuCores = %d, want 2 (inherited from parent)", rt.Requirements.CpuCores)
	}
	if rt.Requirements.MemoryGB != 8 {
		t.Errorf("MemoryGB = %d, want 8 (overridden by child)", rt.Requirements.MemoryGB)
	}
	if rt.Requirements.DiskGB != 10 {
		t.Errorf("DiskGB = %d, want 10 (inherited from parent)", rt.Requirements.DiskGB)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestSelectTarget_MergeExports_ParentAndChild(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"_base": {
			Exports: map[string]step.Overrideable[string]{
				"PATH_A": {Default: "/a"},
				"PATH_B": {Default: "/b-parent"},
			},
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "", true)},
			},
		},
		"linux/*": {
			Base: "_base",
			Exports: map[string]step.Overrideable[string]{
				"PATH_B": {Default: "/b-child"},
				"PATH_C": {Default: "/c"},
			},
			Lifecycle: domain.TargetLifecycle{},
		},
	})
	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Exports["PATH_A"] != "/a" {
		t.Errorf("PATH_A = %q, want /a (inherited from parent)", rt.Exports["PATH_A"])
	}
	if rt.Exports["PATH_B"] != "/b-child" {
		t.Errorf("PATH_B = %q, want /b-child (overridden by child)", rt.Exports["PATH_B"])
	}
	if rt.Exports["PATH_C"] != "/c" {
		t.Errorf("PATH_C = %q, want /c (from child)", rt.Exports["PATH_C"])
	}
}

func TestSelectTarget_MergeMethods_ParentAndChild(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"_base": {
			Methods: map[string]domain.Method{
				"validate": {
					AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
					Steps:       step.StepList{step.NewRunStep("v", "validate.sh", false, "", true)},
				},
			},
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "", true)},
			},
		},
		"linux/*": {
			Base: "_base",
			Methods: map[string]domain.Method{
				"restart": {
					AvailableIn: []domain.ArrowState{domain.ArrowStateRunning},
					Steps:       step.StepList{step.NewRunStep("r", "restart.sh", false, "", true)},
				},
			},
			Lifecycle: domain.TargetLifecycle{},
		},
	})
	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := rt.Methods["validate"]; !ok {
		t.Error("expected validate method inherited from parent")
	}
	if _, ok := rt.Methods["restart"]; !ok {
		t.Error("expected restart method from child")
	}
}

func TestSelectTarget_MergeNamespaces_ParentInherited(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"_base": {
			Tools: []domain.Namespace{
				domain.Namespace("github.com/org/tool"),
			},
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "", true)},
			},
		},
		"linux/*": {
			Base:      "_base",
			Lifecycle: domain.TargetLifecycle{},
		},
	})
	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rt.Tools) != 1 {
		t.Errorf("Tools count = %d, want 1 (inherited from parent)", len(rt.Tools))
	}
}

func TestSelectTarget_MergeNamespaces_EmptyChildOverridesParent(t *testing.T) {
	// A child declaring tools: [] (non-nil empty) must clear the parent's tools,
	// not silently inherit them.
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"_base": {
			Tools: []domain.Namespace{"github.com/org/tool"},
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "", true)},
			},
		},
		"linux/*": {
			Base:      "_base",
			Tools:     []domain.Namespace{}, // explicit empty — must override
			Lifecycle: domain.TargetLifecycle{},
		},
	})
	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rt.Tools) != 0 {
		t.Errorf("Tools = %v, want empty (child declared tools: [])", rt.Tools)
	}
}

func TestSelectTarget_MergeRequirements_PartialOverride(t *testing.T) {
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"_base": {
			Requirements: domain.Requirement{CpuCores: 2, MemoryGB: 4, DiskGB: 10},
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "", true)},
			},
		},
		"linux/*": {
			Base:         "_base",
			Requirements: domain.Requirement{DiskGB: 50},
			Lifecycle:    domain.TargetLifecycle{},
		},
	})
	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Requirements.CpuCores != 2 {
		t.Errorf("CpuCores = %d, want 2 (inherited from parent)", rt.Requirements.CpuCores)
	}
	if rt.Requirements.MemoryGB != 4 {
		t.Errorf("MemoryGB = %d, want 4 (inherited from parent)", rt.Requirements.MemoryGB)
	}
	if rt.Requirements.DiskGB != 50 {
		t.Errorf("DiskGB = %d, want 50 (overridden by child)", rt.Requirements.DiskGB)
	}
}

func TestSelectTarget_BuildResolvedTarget_ExportsAndMethods(t *testing.T) {
	exportCmd := step.Overrideable[string]{
		Default: "/usr/bin/tool",
		OSArch:  map[string]string{"linux/amd64": "/usr/bin/tool-amd64"},
	}
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"linux/*": {
			Exports: map[string]step.Overrideable[string]{
				"BIN": exportCmd,
			},
			Methods: map[string]domain.Method{
				"check": {
					AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
					Steps: step.StepList{
						step.RunStep{
							Command: step.Overrideable[string]{
								Default: "check.sh",
								OSArch:  map[string]string{"linux/amd64": "check-amd64.sh"},
							},
							Timeout: step.Overrideable[string]{Default: "10s"},
						},
					},
				},
			},
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "", true)},
			},
		},
	})
	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Exports["BIN"] != "/usr/bin/tool-amd64" {
		t.Errorf("BIN export = %q, want /usr/bin/tool-amd64 (resolved via OSArch for linux/amd64)", rt.Exports["BIN"])
	}
	checkMethod, ok := rt.Methods["check"]
	if !ok {
		t.Fatal("expected check method in resolved target")
	}
	gotCmd := checkMethod.Steps[0].(step.RunStep).Command.Default
	if gotCmd != "check-amd64.sh" {
		t.Errorf("check method command = %q, want check-amd64.sh", gotCmd)
	}
}

func TestSelectTarget_MergeNamespaces_EmptyChildInheritsParent(t *testing.T) {
	// child does not declare services (nil) → parent services must be inherited
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"_base": {
			Services: []domain.Namespace{"github.com/org/svc"},
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "", true)},
			},
		},
		"linux/*": {
			Base:      "_base",
			Services:  nil, // not declared → inherit from parent
			Lifecycle: domain.TargetLifecycle{},
		},
	})
	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// len(child) == 0 so mergeNamespaces returns parent's services
	if len(rt.Services) != 1 {
		t.Errorf("Services count = %d, want 1 (inherited from parent)", len(rt.Services))
	}
}

func TestSelectTarget_MergeRequirements_AllZeroChildInheritsAll(t *testing.T) {
	// child has all-zero requirements → all parent values must be kept
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"_base": {
			Requirements: domain.Requirement{CpuCores: 4, MemoryGB: 8, DiskGB: 20},
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "", true)},
			},
		},
		"linux/*": {
			Base:         "_base",
			Requirements: domain.Requirement{}, // all zero → parent wins
			Lifecycle:    domain.TargetLifecycle{},
		},
	})
	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Requirements.CpuCores != 4 {
		t.Errorf("CpuCores = %d, want 4 (inherited from parent)", rt.Requirements.CpuCores)
	}
	if rt.Requirements.MemoryGB != 8 {
		t.Errorf("MemoryGB = %d, want 8 (inherited from parent)", rt.Requirements.MemoryGB)
	}
	if rt.Requirements.DiskGB != 20 {
		t.Errorf("DiskGB = %d, want 20 (inherited from parent)", rt.Requirements.DiskGB)
	}
}

func TestSelectTarget_OverrideableResolution(t *testing.T) {
	cmd := step.Overrideable[string]{
		Default: "default-cmd",
		OSArch: map[string]string{
			"linux/amd64": "amd64-cmd",
		},
	}
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"linux/*": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{
					step.RunStep{
						Command:  cmd,
						Elevated: step.Overrideable[bool]{Default: false},
						Timeout:  step.Overrideable[string]{Default: ""},
					},
				},
			},
		},
	})

	rt, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("linux/amd64: unexpected error: %v", err)
	}
	got := rt.Lifecycle.Install[0].(step.RunStep).Command.Default
	if got != "amd64-cmd" {
		t.Fatalf("linux/amd64: expected amd64-cmd, got %q", got)
	}

	rt2, err := v0.SelectTarget(targets, domain.OSLinuxARM64)
	if err != nil {
		t.Fatalf("linux/arm64: unexpected error: %v", err)
	}
	got2 := rt2.Lifecycle.Install[0].(step.RunStep).Command.Default
	if got2 != "default-cmd" {
		t.Fatalf("linux/arm64: expected default-cmd, got %q", got2)
	}
}

func TestSelectTarget_BaseInheritance_OmittedChildLifecycle(t *testing.T) {
	// A child target that declares install: but omits execute:
	// must inherit execute: from the abstract base — not override it with empty.
	// This test directly uses nil to represent the omitted field, testing the
	// mergeStepList logic that checks "child != nil".
	targets := makeTargets(map[string]models.PrecompiledTarget{
		"_base": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{
					step.NewRunStep("base install", "echo base-install", false, "30s", false),
				},
				Execute: step.StepList{
					step.NewRunStep("base execute", "echo base-execute", false, "30s", false),
				},
			},
		},
		"linux/*": {
			Base: "_base",
			Lifecycle: domain.TargetLifecycle{
				// install declared (overrides base), execute deliberately omitted
				Install: step.StepList{
					step.NewRunStep("child install", "echo child-install", false, "30s", false),
				},
				Execute: nil, // nil = not declared, must inherit from _base
			},
		},
	})

	result, err := v0.SelectTarget(targets, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Lifecycle.Execute) == 0 {
		t.Fatal("expected inherited execute steps from _base, got none")
	}
	if len(result.Lifecycle.Install) == 0 {
		t.Fatal("expected child install steps, got none")
	}
	// child install overrides base install
	installCmd := result.Lifecycle.Install[0].(step.RunStep).Command.Default
	if installCmd != "echo child-install" {
		t.Fatalf("expected child install, got %q", installCmd)
	}
	// execute comes from base
	execCmd := result.Lifecycle.Execute[0].(step.RunStep).Command.Default
	if execCmd != "echo base-execute" {
		t.Fatalf("expected base execute, got %q", execCmd)
	}
}
