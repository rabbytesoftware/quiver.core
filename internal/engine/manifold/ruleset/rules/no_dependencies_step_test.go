package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

func TestNoDependenciesStepRule_NoDepsStep(t *testing.T) {
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			"*": {
				Lifecycle: domain.TargetLifecycle{
					Install: domainStep.StepList{
						domainStep.NewRunStep("run", "echo hi", false, "", true),
					},
				},
			},
		},
	}
	errs := NoDependenciesStepRule{}.Validate(m)
	assert.Empty(t, errs)
}

func TestNoDependenciesStepRule_WithDepsStep(t *testing.T) {
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			"*": {
				Lifecycle: domain.TargetLifecycle{
					Install: domainStep.StepList{
						domainStep.NewDependenciesStep("Install Dependencies"),
						domainStep.NewRunStep("run", "echo hi", false, "", true),
					},
				},
			},
		},
	}
	errs := NoDependenciesStepRule{}.Validate(m)
	assert.Len(t, errs, 1)
	assert.Equal(t, "no_dependencies_step", errs[0].Rule)
}
