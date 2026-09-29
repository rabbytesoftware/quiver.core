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
		at := exposeAt{os: os, kind: kind, index: i}
		names = append(names, e.Name)
		errs = append(errs, checkExposeName(at, e.Name)...)
		errs = append(errs, checkExposePath(at, e.Path)...)
		errs = append(errs, checkExposeIcon(at, e.Icon)...)
	}
	field := fmt.Sprintf("targets[%s].expose.%s", os, kind)
	errs = append(errs, checkDuplicates(names, field, "duplicate_name")...)
	return errs
}

type exposeAt struct {
	os    domain.OS
	kind  domain.ExposeKind
	index int
}

func (at exposeAt) err(
	field string,
	rule string,
	format string,
	args ...any,
) aerrors.RuleErrors {
	return aerrors.RuleErrors{{
		Field:   fmt.Sprintf("targets[%s].expose.%s[%d].%s", at.os, at.kind, at.index, field),
		Rule:    rule,
		Message: fmt.Sprintf(format, args...),
	}}
}

func checkExposeName(
	at exposeAt,
	name string,
) aerrors.RuleErrors {
	if exposeNameRe.MatchString(name) {
		return nil
	}
	return at.err("name", "invalid_name", "name %q must match ^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$", name)
}

func checkExposePath(
	at exposeAt,
	path string,
) aerrors.RuleErrors {
	if path == domain.ExposeAuto {
		return nil
	}

	var errs aerrors.RuleErrors
	if !workdirAnchored(path) {
		errs = append(errs, at.err("path", "invalid_path", "path %q must be %q or start with ${INSTALL_PATH} or ${WORKDIR}", path, domain.ExposeAuto)...)
	}
	if containsPathTraversal(path) {
		errs = append(errs, at.err("path", "path_traversal", "path %q must not contain \"..\"", path)...)
	}
	if at.kind == domain.ExposeKindDesktop && at.os.IsDarwin() && !strings.HasSuffix(path, ".app") {
		errs = append(errs, at.err("path", "invalid_desktop_path", "darwin desktop path %q must end in .app", path)...)
	}
	return errs
}

func checkExposeIcon(
	at exposeAt,
	icon string,
) aerrors.RuleErrors {
	if isValidExposeIcon(icon) {
		return nil
	}
	return at.err("icon", "invalid_expose_icon", "icon %q must be empty, an http(s) URL, or start with ${INSTALL_PATH} or ${WORKDIR}", icon)
}

func isValidExposeIcon(
	icon string,
) bool {
	if icon == "" || strings.HasPrefix(icon, "http://") || strings.HasPrefix(icon, "https://") {
		return true
	}
	return icon != domain.ExposeAuto && !containsPathTraversal(icon) && workdirAnchored(icon)
}

func workdirAnchored(
	path string,
) bool {
	return strings.HasPrefix(path, "${INSTALL_PATH}") || strings.HasPrefix(path, "${WORKDIR}")
}
