package metadata

import "strings"

const profilePlaceholder = "{{PROFILE}}"

// expandProfile substitutes the user profile directory into a home template.
// It is OS-independent so the Windows expansion can be tested anywhere.
func expandProfile(template, profileDir string) string {
	return strings.ReplaceAll(template, profilePlaceholder, profileDir)
}
