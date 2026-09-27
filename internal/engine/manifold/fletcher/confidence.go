package fletcher

import (
	"slices"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/picker"
)

const (
	WarningAssumedArch          = "assumed_arch"
	WarningEmulated             = "emulated"
	WarningWindowsExeUnverified = "windows_exe_unverified"
	WarningNameMismatch         = "name_mismatch"
)

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

func assess(
	picks map[domain.OS]picker.Pick,
) (Confidence, []string) {
	var raised []string
	for platform, pick := range picks {
		raised = append(raised, pickWarnings(platform, pick)...)
	}
	warnings := ordered(raised)
	return levelOf(warnings), warnings
}

func levelOf(
	warnings []string,
) Confidence {
	if slices.Contains(warnings, WarningNameMismatch) {
		return ConfidenceLow
	}
	if len(warnings) > 0 {
		return ConfidenceMedium
	}
	return ConfidenceHigh
}

func ordered(
	raised []string,
) []string {
	var warnings []string
	for _, warning := range []string{
		WarningAssumedArch,
		WarningEmulated,
		WarningWindowsExeUnverified,
		WarningNameMismatch,
	} {
		if slices.Contains(raised, warning) {
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

func pickWarnings(
	platform domain.OS,
	pick picker.Pick,
) []string {
	var warnings []string
	if warning := matchWarning(pick.Match); warning != "" {
		warnings = append(warnings, warning)
	}
	if isUnverifiedWindowsExe(platform, pick) {
		warnings = append(warnings, WarningWindowsExeUnverified)
	}
	if !pick.NameMatch {
		warnings = append(warnings, WarningNameMismatch)
	}
	return warnings
}

func matchWarning(
	match picker.Match,
) string {
	switch match {
	case picker.MatchAssumed:
		return WarningAssumedArch
	case picker.MatchEmulated:
		return WarningEmulated
	case picker.MatchExact:
		return ""
	}
	return ""
}

func isUnverifiedWindowsExe(
	platform domain.OS,
	pick picker.Pick,
) bool {
	if !platform.IsWindows() || pick.Format != picker.FormatBinary {
		return false
	}
	return strings.HasSuffix(strings.ToLower(pick.Asset.Name), ".exe")
}
