// Package releasetags_test pins the tag names quiver.core's release workflows
// publish, and proves the channel classifier orders them the way the
// workflows mean them.
package releasetags_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// quiverCoreTags is quiver.core's own tag list on 2026-09-30 (git ls-remote).
var quiverCoreTags = []string{
	"stable-26.5", "stable-26.5.1",
	"beta-26.5", "beta-26.5-1", "beta-26.5-2", "beta-26.5-3", "beta-26.5-4",
	"hotfix-26.5.2", "nightly-latest", "beta-2026-09-27",
}

func releaseTag(t *testing.T, tags []string, args ...string) (string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the release workflows run the script under bash on ubuntu")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", ".github", "scripts", "release-tag.sh"))
	require.NoError(t, err)
	cmd := exec.Command(bash, append([]string{script}, args...)...) // #nosec G204 -- the repository's own script
	cmd.Stdin = strings.NewReader(strings.Join(tags, "\n") + "\n")
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func with(tags []string, more ...string) []string {
	return append(append([]string{}, tags...), more...)
}

func TestReleaseTag_Names(t *testing.T) {
	testCases := []struct {
		name string
		tags []string
		args []string
		want string
	}{
		{name: "first beta of a calendar series", tags: []string{"stable-26.5.1"}, args: []string{"prerelease", "beta/26.6"}, want: "beta-26.6"},
		{name: "second beta of a calendar series", tags: quiverCoreTags, args: []string{"prerelease", "beta/26.5"}, want: "beta-26.5-5"},
		{name: "second beta of a dated series", tags: quiverCoreTags, args: []string{"prerelease", "beta/2026-09-27"}, want: "beta-2026-09-27-1"},
		{name: "hotfix of a calendar stable", tags: quiverCoreTags, args: []string{"prerelease", "hotfix/crash"}, want: "hotfix-26.5.2-1"},
		{name: "stable from a calendar beta", tags: quiverCoreTags, args: []string{"stable", "beta/26.5"}, want: "stable-26.5.2"},
		{name: "stable from the dated beta", tags: quiverCoreTags, args: []string{"stable", "beta/2026-09-27"}, want: "stable-2026-09-27"},
		{name: "hotfix of the dated stable", tags: with(quiverCoreTags, "stable-2026-09-27"), args: []string{"prerelease", "hotfix/crash"}, want: "hotfix-2026-09-27.1"},
		{name: "hotfix rebuilt", tags: with(quiverCoreTags, "stable-2026-09-27", "hotfix-2026-09-27.1"), args: []string{"prerelease", "hotfix/crash"}, want: "hotfix-2026-09-27.1-1"},
		{name: "the dated hotfix promoted", tags: with(quiverCoreTags, "stable-2026-09-27", "hotfix-2026-09-27.1"), args: []string{"stable", "hotfix/crash"}, want: "stable-2026-09-27.1"},
		{name: "the next dated patch", tags: with(quiverCoreTags, "stable-2026-09-27", "stable-2026-09-27.1"), args: []string{"stable", "hotfix/crash"}, want: "stable-2026-09-27.2"},
		{name: "hotfix of a dated patch", tags: with(quiverCoreTags, "stable-2026-09-27", "stable-2026-09-27.1"), args: []string{"prerelease", "hotfix/x"}, want: "hotfix-2026-09-27.2"},
		{name: "a later calendar stable is the hotfix base", tags: with(quiverCoreTags, "stable-2026-09-27", "stable-26.10"), args: []string{"prerelease", "hotfix/x"}, want: "hotfix-26.10.1"},
		{name: "a later date is the hotfix base", tags: with(quiverCoreTags, "stable-2026-09-27.3", "stable-2026-10-01"), args: []string{"prerelease", "hotfix/x"}, want: "hotfix-2026-10-01.1"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := releaseTag(t, tc.tags, tc.args...)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestReleaseTag_Refuses(t *testing.T) {
	testCases := []struct {
		name string
		tags []string
		args []string
	}{
		{name: "a beta branch that names no series", tags: quiverCoreTags, args: []string{"prerelease", "beta/shiny"}},
		{name: "a hotfix with no stable baseline", tags: []string{"beta-26.5"}, args: []string{"prerelease", "hotfix/x"}},
		{name: "an unknown branch", tags: quiverCoreTags, args: []string{"prerelease", "feature/x"}},
		{name: "an unknown mode", tags: quiverCoreTags, args: []string{"nightly", "beta/26.5"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := releaseTag(t, tc.tags, tc.args...)
			assert.Error(t, err)
		})
	}
}
