package assemblerinternal_test

import (
	"testing"

	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	assemblerinternal "github.com/rabbytesoftware/quiver/internal/app/repositories/runtime/internal/assembler/internal"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runStep() domainStep.Step {
	return domainStep.NewRunStep("test", "echo hi", false, "", true)
}

func targetWithInstall() domain.Target {
	return domain.Target{
		Lifecycle: domain.TargetLifecycle{
			Install: domainStep.StepList{runStep()},
		},
	}
}

func TestStepsForMethod_Install(t *testing.T) {
	target := targetWithInstall()
	steps, availableIn, err := assemblerinternal.StepsForMethod(target, domain.MethodInstall)
	require.NoError(t, err)
	// Install prepends a DependenciesStep
	assert.NotEmpty(t, steps)
	assert.Nil(t, availableIn)
}

func TestStepsForMethod_Uninstall(t *testing.T) {
	target := domain.Target{
		Lifecycle: domain.TargetLifecycle{
			Uninstall: domainStep.StepList{runStep()},
		},
	}
	steps, availableIn, err := assemblerinternal.StepsForMethod(target, domain.MethodUninstall)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	assert.Contains(t, availableIn, domain.ArrowStateReady)
}

func TestStepsForMethod_Update_WithSteps(t *testing.T) {
	target := domain.Target{
		Lifecycle: domain.TargetLifecycle{
			Update: domainStep.StepList{runStep()},
		},
	}
	steps, availableIn, err := assemblerinternal.StepsForMethod(target, domain.MethodUpdate)
	require.NoError(t, err)
	assert.NotEmpty(t, steps)
	assert.Contains(t, availableIn, domain.ArrowStateReady)
}

func TestStepsForMethod_Update_NoSteps_MethodNotFound(t *testing.T) {
	target := domain.Target{}
	_, _, err := assemblerinternal.StepsForMethod(target, domain.MethodUpdate)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrMethodNotFound)
}

func TestStepsForMethod_Execute_WithSteps(t *testing.T) {
	target := domain.Target{
		Lifecycle: domain.TargetLifecycle{
			Execute: domainStep.StepList{runStep()},
		},
	}
	steps, availableIn, err := assemblerinternal.StepsForMethod(target, domain.MethodExecute)
	require.NoError(t, err)
	assert.NotEmpty(t, steps)
	assert.Contains(t, availableIn, domain.ArrowStateReady)
}

func TestStepsForMethod_Execute_NoSteps_MethodNotFound(t *testing.T) {
	target := domain.Target{}
	_, _, err := assemblerinternal.StepsForMethod(target, domain.MethodExecute)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrMethodNotFound)
}

func TestStepsForMethod_Stop_WithSteps(t *testing.T) {
	target := domain.Target{
		Lifecycle: domain.TargetLifecycle{
			Stop: domainStep.StepList{runStep()},
		},
	}
	steps, availableIn, err := assemblerinternal.StepsForMethod(target, domain.MethodStop)
	require.NoError(t, err)
	assert.NotEmpty(t, steps)
	assert.Contains(t, availableIn, domain.ArrowStateStopping)
}

func TestStepsForMethod_Stop_NoSteps_MethodNotFound(t *testing.T) {
	target := domain.Target{}
	_, _, err := assemblerinternal.StepsForMethod(target, domain.MethodStop)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrMethodNotFound)
}

func TestStepsForMethod_CustomMethod_Found(t *testing.T) {
	target := domain.Target{
		Methods: map[string]domain.Method{
			"deploy": {
				Steps:       domainStep.StepList{runStep()},
				AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
			},
		},
	}
	steps, availableIn, err := assemblerinternal.StepsForMethod(target, "deploy")
	require.NoError(t, err)
	assert.NotEmpty(t, steps)
	assert.Contains(t, availableIn, domain.ArrowStateReady)
}

func TestStepsForMethod_CustomMethod_NotFound(t *testing.T) {
	target := domain.Target{}
	_, _, err := assemblerinternal.StepsForMethod(target, "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrMethodNotFound)
}

func TestStepsForMethod_CustomMethod_EmptySteps(t *testing.T) {
	target := domain.Target{
		Methods: map[string]domain.Method{
			"deploy": {Steps: domainStep.StepList{}},
		},
	}
	_, _, err := assemblerinternal.StepsForMethod(target, "deploy")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrMethodNotFound)
}
