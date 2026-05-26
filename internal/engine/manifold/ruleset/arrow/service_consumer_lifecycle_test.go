package arrow

import (
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func TestServiceConsumerLifecycleRule_WithServicesAndBothSteps_NoError(t *testing.T) {
	rule := ServiceConsumerLifecycleRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Services: []domain.DependencyEdge{{Type: domain.ServiceDep}},
				Lifecycle: domain.TargetLifecycle{
					Execute: step.StepList{step.RunStep{}},
					Stop:    step.StepList{step.RunStep{}},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestServiceConsumerLifecycleRule_WithServicesMissingExecute_Error(t *testing.T) {
	rule := ServiceConsumerLifecycleRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Services: []domain.DependencyEdge{{Type: domain.ServiceDep}},
				Lifecycle: domain.TargetLifecycle{
					Stop: step.StepList{step.RunStep{}},
					// Execute absent
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing execute, got none")
	}
	if errs[0].Rule != "service_consumer_missing_lifecycle" {
		t.Fatalf("expected rule %q, got %q", "service_consumer_missing_lifecycle", errs[0].Rule)
	}
}

func TestServiceConsumerLifecycleRule_WithServicesMissingStop_Error(t *testing.T) {
	rule := ServiceConsumerLifecycleRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Services: []domain.DependencyEdge{{Type: domain.ServiceDep}},
				Lifecycle: domain.TargetLifecycle{
					Execute: step.StepList{step.RunStep{}},
					// Stop absent
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing stop, got none")
	}
	if errs[0].Rule != "service_consumer_missing_lifecycle" {
		t.Fatalf("expected rule %q, got %q", "service_consumer_missing_lifecycle", errs[0].Rule)
	}
}

func TestServiceConsumerLifecycleRule_WithServicesMissingBoth_Error(t *testing.T) {
	rule := ServiceConsumerLifecycleRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Services:  []domain.DependencyEdge{{Type: domain.ServiceDep}},
				Lifecycle: domain.TargetLifecycle{
					// Execute and Stop both absent
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing execute and stop, got none")
	}
	if errs[0].Rule != "service_consumer_missing_lifecycle" {
		t.Fatalf("expected rule %q, got %q", "service_consumer_missing_lifecycle", errs[0].Rule)
	}
}

func TestServiceConsumerLifecycleRule_MultiTarget_OnlyFailingTargetErrors(t *testing.T) {
	rule := ServiceConsumerLifecycleRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				// target1: has services, has execute+stop → valid
				Services: []domain.DependencyEdge{{Type: domain.ServiceDep}},
				Lifecycle: domain.TargetLifecycle{
					Execute: step.StepList{step.RunStep{}},
					Stop:    step.StepList{step.RunStep{}},
				},
			},
			domain.OSDarwinAMD64: {
				// target2: has services, has execute but no stop → error
				Services: []domain.DependencyEdge{{Type: domain.ServiceDep}},
				Lifecycle: domain.TargetLifecycle{
					Execute: step.StepList{step.RunStep{}},
					// Stop absent
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), errs)
	}
	expectedField := "targets[darwin/amd64].lifecycle"
	if errs[0].Field != expectedField {
		t.Fatalf("expected field %q, got %q", expectedField, errs[0].Field)
	}
}

func TestServiceConsumerLifecycleRule_NoServices_NoError(t *testing.T) {
	rule := ServiceConsumerLifecycleRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				// No services — execute/stop not required
				Lifecycle: domain.TargetLifecycle{},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for target with no services, got: %v", errs)
	}
}
