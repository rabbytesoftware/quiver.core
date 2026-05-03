package assemblerinternal

import (
	"fmt"

	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

func StepsForMethod(
	target domain.Target,
	method string,
) ([]domainStep.Step, []domain.ArrowState, error) {
	switch method {
	case domain.MethodInstall:
		depStep := domainStep.NewDependenciesStep("Resolve dependencies")
		installSteps := []domainStep.Step{depStep}
		installSteps = append(installSteps, target.Lifecycle.Install...)
		return installSteps, nil, nil

	case domain.MethodUninstall:
		return target.Lifecycle.Uninstall,
			[]domain.ArrowState{domain.ArrowStateReady},
			nil

	case domain.MethodUpdate:
		if len(target.Lifecycle.Update) == 0 {
			return nil,
				nil,
				fmt.Errorf("stepsForMethod: %w", apperrors.ErrMethodNotFound)
		}
		return target.Lifecycle.Update,
			[]domain.ArrowState{domain.ArrowStateReady},
			nil

	case domain.MethodExecute:
		if len(target.Lifecycle.Execute) == 0 {
			return nil,
				nil,
				fmt.Errorf("stepsForMethod: %w", apperrors.ErrMethodNotFound)
		}
		return target.Lifecycle.Execute,
			[]domain.ArrowState{domain.ArrowStateReady},
			nil

	case domain.MethodStop:
		if len(target.Lifecycle.Stop) == 0 {
			return nil,
				nil,
				fmt.Errorf("stepsForMethod: %w", apperrors.ErrMethodNotFound)
		}
		return target.Lifecycle.Stop,
			[]domain.ArrowState{domain.ArrowStateStopping},
			nil

	default:
		m, ok := target.Methods[method]
		if !ok || len(m.Steps) == 0 {
			return nil,
				nil,
				fmt.Errorf("stepsForMethod: %w", apperrors.ErrMethodNotFound)
		}
		return m.Steps,
			m.AvailableIn,
			nil
	}
}
