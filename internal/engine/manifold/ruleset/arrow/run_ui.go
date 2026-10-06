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

// RunUIRule validates the ui node of run steps: one source, a known
// transport, a local static dir, no ui in preinstalled, and
// ${ARROW_UI_LISTEN} only in the command of a run whose own ui listens.
type RunUIRule struct{}

func (RunUIRule) Name() string { return "run_ui" }

func (RunUIRule) Validate(
	m *domain.Arrow,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for os, target := range m.Targets {
		for _, g := range runUIGroups(target) {
			errs = append(errs, checkRunUIGroup(string(os), g.name, g.steps)...)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

type runUIGroup struct {
	name  string
	steps domainStep.StepList
}

func runUIGroups(
	t domain.Target,
) []runUIGroup {
	groups := []runUIGroup{
		{"lifecycle.install", t.Lifecycle.Install},
		{"lifecycle.update", t.Lifecycle.Update},
		{"lifecycle.execute", t.Lifecycle.Execute},
		{"lifecycle.stop", t.Lifecycle.Stop},
		{"lifecycle.uninstall", t.Lifecycle.Uninstall},
		{preinstalledGroup, t.Lifecycle.Preinstalled},
	}
	for name, method := range t.Methods {
		groups = append(groups, runUIGroup{"methods." + name + ".steps", method.Steps})
	}
	return groups
}

func checkRunUIGroup(
	key string,
	name string,
	steps domainStep.StepList,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for i, s := range steps {
		run, ok := s.(domainStep.RunStep)
		if !ok {
			continue
		}
		field := fmt.Sprintf("targets[%s].%s[%d]", key, name, i)
		if overrideableMentions(run.Command, uiListenVarRef) && (run.UI == nil || !run.UI.Listens()) {
			errs = append(errs, runUIError(field, uiListenVarRef+" is only available in the command of a run whose ui listens"))
		}
		if run.UI == nil {
			continue
		}
		if name == preinstalledGroup {
			errs = append(errs, runUIError(field, "ui is not allowed in preinstalled: a probe opens no surface"))
			continue
		}
		errs = append(errs, checkRunUISource(field, *run.UI)...)
	}
	return errs
}

func checkRunUISource(
	field string,
	ui domainStep.UIOptions,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	switch {
	case len(ui.Listen) > 0 && ui.Static != "":
		errs = append(errs, runUIError(field, "listen and static are mutually exclusive"))
	case ui.Listens():
		errs = append(errs, checkRunUIListen(field, ui.Listen)...)
	case !filepath.IsLocal(ui.Static):
		errs = append(errs, runUIError(field, "static must be a relative path inside the arrow's install directory"))
	}
	if ui.Path != "" && !strings.HasPrefix(ui.Path, "/") {
		errs = append(errs, runUIError(field, "path must start with /"))
	}
	return errs
}

func checkRunUIListen(
	field string,
	listen []string,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for _, kind := range listen {
		if !slices.Contains(uiListenKinds, kind) {
			errs = append(errs, runUIError(field, fmt.Sprintf("unknown listen kind %q (allowed: unix, pipe)", kind)))
		}
	}
	if slices.Contains(listen, "pipe") && !slices.Contains(listen, "unix") {
		errs = append(errs, runUIError(field, "pipe is not provisioned in v0; include unix"))
	}
	return errs
}

func runUIError(
	field string,
	msg string,
) aerrors.RuleError {
	return aerrors.RuleError{Field: field, Rule: "run_ui", Message: msg}
}

func overrideableMentions(
	o domainStep.Overrideable[string],
	needle string,
) bool {
	return slices.ContainsFunc(overrideableValues(o), func(v string) bool {
		return strings.Contains(v, needle)
	})
}
