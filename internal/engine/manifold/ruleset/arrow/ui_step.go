package arrow

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

var uiListenKinds = []string{"unix", "pipe"}

const (
	uiListenVarRef    = "${" + domain.VarArrowUIListen + "}"
	preinstalledGroup = "lifecycle.preinstalled"
)

// UIStepRule validates ui steps: one source, a known transport, a local
// static dir, no ui in preinstalled, and ${ARROW_UI_LISTEN} only after a
// listening ui step.
type UIStepRule struct{}

func (UIStepRule) Name() string { return "ui_step" }

func (UIStepRule) Validate(
	m *domain.Arrow,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for os, target := range m.Targets {
		for _, g := range uiStepGroups(target) {
			errs = append(errs, checkUIGroup(string(os), g.name, g.steps)...)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

type uiStepGroup struct {
	name  string
	steps domainStep.StepList
}

func uiStepGroups(
	t domain.Target,
) []uiStepGroup {
	groups := []uiStepGroup{
		{"lifecycle.install", t.Lifecycle.Install},
		{"lifecycle.update", t.Lifecycle.Update},
		{"lifecycle.execute", t.Lifecycle.Execute},
		{"lifecycle.stop", t.Lifecycle.Stop},
		{"lifecycle.uninstall", t.Lifecycle.Uninstall},
		{preinstalledGroup, t.Lifecycle.Preinstalled},
	}
	for name, method := range t.Methods {
		groups = append(groups, uiStepGroup{"methods." + name + ".steps", method.Steps})
	}
	return groups
}

func checkUIGroup(
	key string,
	name string,
	steps domainStep.StepList,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	uiSeen, listenSeen := false, false
	for i, s := range steps {
		field := fmt.Sprintf("targets[%s].%s[%d]", key, name, i)
		switch st := s.(type) {
		case domainStep.UIStep:
			if name == preinstalledGroup {
				errs = append(errs, uiError(field, "ui steps are not allowed in preinstalled: a probe opens no surface"))
				continue
			}
			if uiSeen {
				errs = append(errs, uiError(field, "at most one ui step is allowed per method"))
			}
			uiSeen = true
			listenSeen = listenSeen || len(st.Listen) > 0
			errs = append(errs, checkUISource(field, st)...)
		case domainStep.RunStep:
			if overrideableMentions(st.Command, uiListenVarRef) && !listenSeen {
				errs = append(errs, uiError(field, uiListenVarRef+" is only available after a ui step with listen in the same method"))
			}
		}
	}
	return errs
}

func checkUISource(
	field string,
	st domainStep.UIStep,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	hasListen, hasStatic := len(st.Listen) > 0, st.Static != ""
	switch {
	case hasListen == hasStatic:
		errs = append(errs, uiError(field, "exactly one of listen or static is required"))
	case hasListen:
		for _, kind := range st.Listen {
			if !slices.Contains(uiListenKinds, kind) {
				errs = append(errs, uiError(field, fmt.Sprintf("unknown listen kind %q (allowed: unix, pipe)", kind)))
			}
		}
		if slices.Contains(st.Listen, "pipe") && !slices.Contains(st.Listen, "unix") {
			errs = append(errs, uiError(field, "pipe is not provisioned in v0; include unix"))
		}
	case !filepath.IsLocal(st.Static):
		errs = append(errs, uiError(field, "static must be a relative path inside the arrow's install directory"))
	}
	if st.Path != "" && !strings.HasPrefix(st.Path, "/") {
		errs = append(errs, uiError(field, "path must start with /"))
	}
	return errs
}

func uiError(
	field string,
	msg string,
) aerrors.RuleError {
	return aerrors.RuleError{Field: field, Rule: "ui_step", Message: msg}
}

func overrideableMentions(
	o domainStep.Overrideable[string],
	needle string,
) bool {
	return slices.ContainsFunc(overrideableValues(o), func(v string) bool {
		return strings.Contains(v, needle)
	})
}
