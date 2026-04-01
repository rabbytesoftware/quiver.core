package assembler_test

import (
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/assembler"
)

func TestValidateArrow_Valid(t *testing.T) {
	manifest := &domain.ArrowManifest{
		Name:    "test-arrow",
		Version: "1.0.0",
		Requirements: domain.Requirement{
			CpuCores: 2,
			MemoryGB: 4,
			DiskGB:   10,
			OS:       []domain.OS{domain.OS("linux/amd64")},
		},
		Lifecycle: domain.Lifecycle{
			Install: step.StepList{
				step.NewRunStep("Install", "./install.sh", 0, true),
			},
			Uninstall: step.StepList{
				step.NewRunStep("Uninstall", "./uninstall.sh", 0, true),
			},
		},
	}
	if err := assembler.ValidateArrow(manifest); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateArrow_DuplicateVariables(t *testing.T) {
	manifest := &domain.ArrowManifest{
		Name:    "test-arrow",
		Version: "1.0.0",
		Requirements: domain.Requirement{
			CpuCores: 2,
			MemoryGB: 4,
			DiskGB:   10,
		},
		Lifecycle: domain.Lifecycle{
			Install: step.StepList{
				step.NewRunStep("Install", "./install.sh", 0, true),
			},
			Uninstall: step.StepList{
				step.NewRunStep("Uninstall", "./uninstall.sh", 0, true),
			},
		},
		Variables: []domain.Variable{
			{Name: "VAR1"},
			{Name: "VAR1"},
		},
	}
	if err := assembler.ValidateArrow(manifest); err == nil {
		t.Fatal("expected error for duplicate variables")
	}
}

func TestValidateArrow_MissingUninstall(t *testing.T) {
	manifest := &domain.ArrowManifest{
		Name:    "test-arrow",
		Version: "1.0.0",
		Requirements: domain.Requirement{
			CpuCores: 2,
			MemoryGB: 4,
			DiskGB:   10,
		},
		Lifecycle: domain.Lifecycle{
			Install: step.StepList{
				step.NewRunStep("Install", "./install.sh", 0, true),
			},
		},
	}
	if err := assembler.ValidateArrow(manifest); err == nil {
		t.Fatal("expected error for missing uninstall")
	}
}

func TestValidateQuiver_Valid(t *testing.T) {
	manifest := &domain.QuiverManifest{
		Name:        "test-quiver",
		Description: "A test quiver",
	}
	if err := assembler.ValidateQuiver(manifest); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateQuiver_EmptyName(t *testing.T) {
	manifest := &domain.QuiverManifest{
		Name: "",
	}
	if err := assembler.ValidateQuiver(manifest); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateArrow_DuplicateNetbridgePorts(t *testing.T) {
	manifest := &domain.ArrowManifest{
		Name:    "test-arrow",
		Version: "1.0.0",
		Requirements: domain.Requirement{
			CpuCores: 1,
			MemoryGB: 1,
			DiskGB:   1,
		},
		Lifecycle: domain.Lifecycle{
			Install: step.StepList{
				step.NewRunStep("Install", "./install.sh", 0, true),
			},
			Uninstall: step.StepList{
				step.NewRunStep("Uninstall", "./uninstall.sh", 0, true),
			},
		},
		Netbridge: []netbridge.PortDef{
			{Name: "PORT1", Protocol: netbridge.Protocol("tcp"), Default: 8080},
			{Name: "PORT1", Protocol: netbridge.Protocol("tcp"), Default: 9090},
		},
	}
	if err := assembler.ValidateArrow(manifest); err == nil {
		t.Fatal("expected error for duplicate netbridge ports")
	}
}

func TestValidateArrow_InvalidMethodAvailableState(t *testing.T) {
	manifest := &domain.ArrowManifest{
		Name:    "test-arrow",
		Version: "1.0.0",
		Requirements: domain.Requirement{
			CpuCores: 1,
			MemoryGB: 1,
			DiskGB:   1,
		},
		Lifecycle: domain.Lifecycle{
			Install: step.StepList{
				step.NewRunStep("Install", "./install.sh", 0, true),
			},
			Uninstall: step.StepList{
				step.NewRunStep("Uninstall", "./uninstall.sh", 0, true),
			},
		},
		Methods: map[string]domain.Method{
			"test": {
				AvailableIn: []domain.ArrowState{domain.ArrowState("invalid_state")},
				Steps:       step.StepList{},
			},
		},
	}
	if err := assembler.ValidateArrow(manifest); err == nil {
		t.Fatal("expected error for invalid method available state")
	}
}

func TestValidateArrow_MissingExecuteWithoutStop(t *testing.T) {
	manifest := &domain.ArrowManifest{
		Name:    "test-arrow",
		Version: "1.0.0",
		Requirements: domain.Requirement{
			CpuCores: 1,
			MemoryGB: 1,
			DiskGB:   1,
		},
		Lifecycle: domain.Lifecycle{
			Install: step.StepList{
				step.NewRunStep("Install", "./install.sh", 0, true),
			},
			Uninstall: step.StepList{
				step.NewRunStep("Uninstall", "./uninstall.sh", 0, true),
			},
			Execute: step.StepList{
				step.NewRunStep("Run", "./start.sh", 0, true),
			},
		},
	}
	if err := assembler.ValidateArrow(manifest); err == nil {
		t.Fatal("expected error for execute without stop")
	}
}

func TestValidateArrow_ValidWithFullLifecycle(t *testing.T) {
	manifest := &domain.ArrowManifest{
		Name:    "test-arrow",
		Version: "1.0.0",
		Requirements: domain.Requirement{
			CpuCores: 2,
			MemoryGB: 4,
			DiskGB:   10,
		},
		Lifecycle: domain.Lifecycle{
			Install: step.StepList{
				step.NewRunStep("Install", "./install.sh", 0, true),
			},
			Uninstall: step.StepList{
				step.NewRunStep("Uninstall", "./uninstall.sh", 0, true),
			},
			Execute: step.StepList{
				step.NewRunStep("Execute", "./start.sh", 0, false),
			},
			Stop: step.StepList{
				step.NewSignalStep("Stop", "SIGTERM", 30*time.Second, true),
			},
		},
	}
	if err := assembler.ValidateArrow(manifest); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
