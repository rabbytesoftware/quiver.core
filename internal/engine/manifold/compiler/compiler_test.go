package compiler_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/compiler"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	v0 "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator/arrow/v0"
)

func TestCompile_CatchAllTarget_ReturnsAllSixOSes(t *testing.T) {
	manifest := &domain.Arrow{}
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{
					step.NewRunStep("install", "echo ok", false, "10s", true),
				},
				Uninstall: step.StepList{},
			},
		},
	}

	err := compiler.New().Compile(manifest, precompiled, v0.New().Selector())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	all := domain.AllOS()
	if len(manifest.Targets) != len(all) {
		t.Fatalf("expected %d entries, got %d", len(all), len(manifest.Targets))
	}
	for _, os := range all {
		if _, ok := manifest.Targets[os]; !ok {
			t.Errorf("missing entry for OS %s", os)
		}
	}
}

func TestCompile_NoMatchingTarget_ReturnsEmptyMap(t *testing.T) {
	manifest := &domain.Arrow{}
	precompiled := map[string]models.PrecompiledTarget{
		"_abstract": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{
					step.NewRunStep("install", "echo ok", false, "10s", true),
				},
				Uninstall: step.StepList{},
			},
		},
	}

	err := compiler.New().Compile(manifest, precompiled, v0.New().Selector())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifest.Targets) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(manifest.Targets))
	}
}

func TestCompile_BaseCycle_ReturnsError(t *testing.T) {
	manifest := &domain.Arrow{}
	precompiled := map[string]models.PrecompiledTarget{
		"linux/*": {
			Base: "_a",
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
				Uninstall: step.StepList{},
			},
		},
		"_a": {Base: "_b"},
		"_b": {Base: "_a"},
	}

	err := compiler.New().Compile(manifest, precompiled, v0.New().Selector())
	if err == nil {
		t.Fatal("expected error for base cycle, got nil")
	}
}

func TestCompile_AmbiguousTarget_ReturnsError(t *testing.T) {
	manifest := &domain.Arrow{}
	precompiled := map[string]models.PrecompiledTarget{
		"linux/*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
				Uninstall: step.StepList{},
			},
		},
		"*/amd64": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
				Uninstall: step.StepList{},
			},
		},
	}

	err := compiler.New().Compile(manifest, precompiled, v0.New().Selector())
	if err == nil {
		t.Fatal("expected error for ambiguous targets, got nil")
	}
}

func TestCompile_DesktopExposeWithoutExecute_GetsStartStop(t *testing.T) {
	manifest := &domain.Arrow{}
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
				Uninstall: step.StepList{},
			},
			Expose: domain.Expose{Desktop: []domain.ExposeEntry{{Name: "app", Path: domain.ExposeAuto}}},
		},
	}

	if err := compiler.New().Compile(manifest, precompiled, v0.New().Selector()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for os, target := range manifest.Targets {
		if len(target.Lifecycle.Execute) != 1 || len(target.Lifecycle.Stop) != 1 {
			t.Errorf("%s: want one execute and one stop, got %d and %d", os, len(target.Lifecycle.Execute), len(target.Lifecycle.Stop))
		}
	}
}

func TestCompile_ExplicitExecuteWins(t *testing.T) {
	manifest := &domain.Arrow{}
	explicit := step.NewRunStep("mine", "./mine", false, "10s", true)
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
				Uninstall: step.StepList{},
				Execute:   step.StepList{explicit},
			},
			Expose: domain.Expose{Desktop: []domain.ExposeEntry{{Name: "app", Path: domain.ExposeAuto}}},
		},
	}

	if err := compiler.New().Compile(manifest, precompiled, v0.New().Selector()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for os, target := range manifest.Targets {
		if len(target.Lifecycle.Execute) != 1 || target.Lifecycle.Execute[0].Title() != explicit.Title() || len(target.Lifecycle.Stop) != 0 {
			t.Errorf("%s: explicit execute was replaced", os)
		}
	}
}
