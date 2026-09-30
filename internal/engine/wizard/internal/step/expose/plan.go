package expose

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func Plan(
	method string,
	expose domain.Expose,
	mediaIcon string,
	steps []domainstep.Step,
) []domainstep.Step {
	if expose.IsEmpty() {
		return steps
	}

	switch method {
	case domain.MethodInstall, domain.MethodUpdate:
		planned := append([]domainstep.Step{}, steps...)
		planned = appendExposeSteps(planned, domain.ExposeKindDesktop, expose.Desktop, mediaIcon)
		return appendExposeSteps(planned, domain.ExposeKindCLI, expose.CLI, mediaIcon)
	case domain.MethodUninstall:
		return append([]domainstep.Step{domainstep.NewUnexposeStep()}, steps...)
	}
	return steps
}

func appendExposeSteps(
	steps []domainstep.Step,
	kind domain.ExposeKind,
	entries []domain.ExposeEntry,
	mediaIcon string,
) []domainstep.Step {
	for _, e := range entries {
		s := domainstep.NewExposeStep(string(kind), e.Name, e.Path)
		s.Icon = e.Icon
		s.Categories = e.Categories
		s.MediaIcon = mediaIcon
		steps = append(steps, s)
	}
	return steps
}
