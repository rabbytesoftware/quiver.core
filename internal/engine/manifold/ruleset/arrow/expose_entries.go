package arrow

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

var exposeNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$`)

type ExposeEntriesRule struct{}

func (ExposeEntriesRule) Name() string { return "expose_entries" }

func (ExposeEntriesRule) Validate(
	m *domain.Arrow,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for os, target := range m.Targets {
		errs = append(errs, checkExpose(os, target)...)
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func checkExpose(
	os domain.OS,
	target domain.Target,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	errs = append(errs, checkExposeEntries(os, domain.ExposeKindCLI, target.Expose.CLI)...)
	errs = append(errs, checkExposeEntries(os, domain.ExposeKindDesktop, target.Expose.Desktop)...)
	return errs
}

func checkExposeEntries(
	os domain.OS,
	kind domain.ExposeKind,
	entries []domain.ExposeEntry,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	names := make([]string, 0, len(entries))
	for i, e := range entries {
		names = append(names, e.Name)
		errs = append(errs, checkExposeName(os, kind, i, e.Name)...)
		errs = append(errs, checkExposePath(os, kind, i, e.Path)...)
		errs = append(errs, checkExposeIcon(os, kind, i, e.Icon)...)
	}
	field := fmt.Sprintf("targets[%s].expose.%s", os, kind)
	errs = append(errs, checkDuplicates(names, field, "duplicate_name")...)
	return errs
}

func checkExposeName(
	os domain.OS,
	kind domain.ExposeKind,
	i int,
	name string,
) aerrors.RuleErrors {
	if exposeNameRe.MatchString(name) {
		return nil
	}
	return aerrors.RuleErrors{{
		Field:   fmt.Sprintf("targets[%s].expose.%s[%d].name", os, kind, i),
		Rule:    "invalid_name",
		Message: fmt.Sprintf("name %q must match ^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$", name),
	}}
}

func checkExposePath(
	os domain.OS,
	kind domain.ExposeKind,
	i int,
	path string,
) aerrors.RuleErrors {
	if path == domain.ExposeAuto {
		return nil
	}

	var errs aerrors.RuleErrors
	errs = append(errs, checkExposePathPrefix(os, kind, i, path)...)
	errs = append(errs, checkExposePathTraversal(os, kind, i, path)...)
	if kind == domain.ExposeKindDesktop && os.IsDarwin() {
		errs = append(errs, checkExposeDesktopSuffix(os, kind, i, path)...)
	}
	return errs
}

func checkExposePathPrefix(
	os domain.OS,
	kind domain.ExposeKind,
	i int,
	path string,
) aerrors.RuleErrors {
	if strings.HasPrefix(path, "${INSTALL_PATH}") || strings.HasPrefix(path, "${WORKDIR}") {
		return nil
	}
	return aerrors.RuleErrors{{
		Field:   fmt.Sprintf("targets[%s].expose.%s[%d].path", os, kind, i),
		Rule:    "invalid_path",
		Message: fmt.Sprintf("path %q must be %q or start with ${INSTALL_PATH} or ${WORKDIR}", path, domain.ExposeAuto),
	}}
}

func checkExposePathTraversal(
	os domain.OS,
	kind domain.ExposeKind,
	i int,
	path string,
) aerrors.RuleErrors {
	if !containsPathTraversal(path) {
		return nil
	}
	return aerrors.RuleErrors{{
		Field:   fmt.Sprintf("targets[%s].expose.%s[%d].path", os, kind, i),
		Rule:    "path_traversal",
		Message: fmt.Sprintf("path %q must not contain \"..\"", path),
	}}
}

func checkExposeDesktopSuffix(
	os domain.OS,
	kind domain.ExposeKind,
	i int,
	path string,
) aerrors.RuleErrors {
	if strings.HasSuffix(path, ".app") {
		return nil
	}
	return aerrors.RuleErrors{{
		Field:   fmt.Sprintf("targets[%s].expose.%s[%d].path", os, kind, i),
		Rule:    "invalid_desktop_path",
		Message: fmt.Sprintf("darwin desktop path %q must end in .app", path),
	}}
}

func checkExposeIcon(
	os domain.OS,
	kind domain.ExposeKind,
	i int,
	icon string,
) aerrors.RuleErrors {
	if isValidExposeIcon(icon) {
		return nil
	}
	return aerrors.RuleErrors{{
		Field:   fmt.Sprintf("targets[%s].expose.%s[%d].icon", os, kind, i),
		Rule:    "invalid_expose_icon",
		Message: fmt.Sprintf("icon %q must be empty, an http(s) URL, or start with ${INSTALL_PATH} or ${WORKDIR}", icon),
	}}
}

func isValidExposeIcon(
	icon string,
) bool {
	if icon == "" {
		return true
	}
	if strings.HasPrefix(icon, "http://") || strings.HasPrefix(icon, "https://") {
		return true
	}
	if icon == domain.ExposeAuto {
		return false
	}
	if containsPathTraversal(icon) {
		return false
	}
	return strings.HasPrefix(icon, "${INSTALL_PATH}") || strings.HasPrefix(icon, "${WORKDIR}")
}
