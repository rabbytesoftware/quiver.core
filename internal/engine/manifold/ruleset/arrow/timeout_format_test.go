package arrow

import (
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func TestTimeoutFormatRule_Valid(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "echo ok", false, "30s", true),
					},
					Uninstall: step.StepList{
						step.NewRunStep("uninstall", "echo bye", false, "5m", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestTimeoutFormatRule_InvalidTimeout(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "echo ok", false, "5minutes", true),
					},
					Uninstall: step.StepList{},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for invalid timeout format, got none")
	}
	if errs[0].Rule != "invalid_timeout" {
		t.Fatalf("expected rule %q, got %q", "invalid_timeout", errs[0].Rule)
	}
}

func TestTimeoutFormatRule_EmptyTimeout(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "", true)},
					Uninstall: step.StepList{},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty timeout, got: %v", errs)
	}
}

func TestTimeoutFormatRule_DependenciesStep_NoTimeout(t *testing.T) {
	// DependenciesStep has no timeout field; extractStepTimeout should return ""
	rule := TimeoutFormatRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{step.NewDependenciesStep("install deps")},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for DependenciesStep (no timeout), got: %v", errs)
	}
}

func TestTimeoutFormatRule_MethodSteps_InvalidTimeout(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
					Uninstall: step.StepList{},
				},
				Methods: map[string]domain.Method{
					"check": {
						AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
						Steps:       step.StepList{step.NewRunStep("check", "check.sh", false, "2h", true)},
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for invalid method step timeout, got none")
	}
	if errs[0].Rule != "invalid_timeout" {
		t.Fatalf("expected rule %q, got %q", "invalid_timeout", errs[0].Rule)
	}
}

func TestTimeoutFormatRule_EmptyTargets(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.Arrow{}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty targets, got: %v", errs)
	}
}

func TestTimeoutFormatRule_FetchStep_ValidTimeout(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("fetch", "https://example.com/file.tar.gz", "/tmp/file.tar.gz", "", "1m", true),
					},
					Uninstall: step.StepList{},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for valid FetchStep timeout, got: %v", errs)
	}
}

func TestTimeoutFormatRule_FetchStep_InvalidTimeout(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("fetch", "https://example.com/file.tar.gz", "/tmp/file.tar.gz", "", "invalid", true),
					},
					Uninstall: step.StepList{},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for invalid FetchStep timeout")
	}
}

func TestTimeoutFormatRule_SignalStep_ValidTimeout(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Stop: step.StepList{
						step.NewSignalStep("stop", "SIGTERM", "30s", true),
					},
					Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
					Uninstall: step.StepList{},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for valid SignalStep timeout, got: %v", errs)
	}
}

func TestTimeoutFormatRule_SignalStep_InvalidTimeout(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Stop: step.StepList{
						step.NewSignalStep("stop", "SIGTERM", "not-a-duration", true),
					},
					Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
					Uninstall: step.StepList{},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for invalid SignalStep timeout")
	}
}

func TestTimeoutFormatRule_AllLifecyclePhases(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "1m", true)},
					Update:    step.StepList{step.NewRunStep("update", "echo upd", false, "2m", true)},
					Execute:   step.StepList{step.NewRunStep("exec", "echo go", false, "5m", true)},
					Stop:      step.StepList{step.NewRunStep("stop", "echo stop", false, "30s", true)},
					Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "1m", true)},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for all valid lifecycle phases, got: %v", errs)
	}
}
