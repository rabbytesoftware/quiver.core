// Package releasetags_test pins the tag names quiver.core's release workflows
// publish, and proves the channel classifier orders them the way the
// workflows mean them.
package releasetags_test

import (
	"os"
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
	bash, err := exec.LookPath("bash")
	if err != nil || runtime.GOOS == "windows" {
		t.Skip("the release workflows run the script under bash on ubuntu")
	}
	return releaseTagWith(t, bash, tags, args...)
}

func releaseTagWith(t *testing.T, bash string, tags []string, args ...string) (string, error) {
	t.Helper()
	script, err := filepath.Abs(filepath.Join("..", "..", ".github", "scripts", "release-tag.sh"))
	require.NoError(t, err)
	cmd := exec.Command(bash, append([]string{script}, args...)...) // #nosec G204 -- the repository's own script
	cmd.Stdin = strings.NewReader(strings.Join(tags, "\n") + "\n")
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// calendarOnly is quiver.core's tag list before the dated beta existed.
var calendarOnly = []string{
	"stable-26.5", "stable-26.5.1",
	"beta-26.5", "beta-26.5-1", "beta-26.5-2", "beta-26.5-3", "beta-26.5-4",
	"hotfix-26.5.2", "nightly-latest",
}

// november is a calendar series with three hotfixes promoted.
var november = []string{
	"beta-26.11", "stable-26.11", "hotfix-26.11.1", "stable-26.11.1",
	"hotfix-26.11.2", "stable-26.11.2", "hotfix-26.11.3", "stable-26.11.3",
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
		{name: "second beta of a calendar series", tags: calendarOnly, args: []string{"prerelease", "beta/26.5"}, want: "beta-26.5-5"},
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
		{name: "a dated beta ranking below the month's calendar patches", tags: november, args: []string{"prerelease", "beta/2026-11-02"}},
		{name: "its stable, had the beta been cut anyway", tags: with(november, "beta-2026-11-02"), args: []string{"stable", "beta/2026-11-02"}},
		{name: "a late calendar beta after a newer series", tags: with(calendarOnly, "stable-26.11"), args: []string{"prerelease", "beta/26.5.3"}},
		{name: "a late calendar stable after a newer series", tags: with(calendarOnly, "stable-26.11"), args: []string{"stable", "beta/26.5.3"}},
		{name: "a calendar beta older than the dated stable", tags: with(quiverCoreTags, "stable-2026-09-27"), args: []string{"prerelease", "beta/26.6"}},
		{name: "a rebuild of an older beta series", tags: quiverCoreTags, args: []string{"prerelease", "beta/26.5"}},
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

// Calendar components written with a leading zero are decimal, never octal.
func TestReleaseTag_ZeroPaddedCalendarSeries(t *testing.T) {
	testCases := []struct {
		name string
		tags []string
		args []string
		want string
	}{
		{name: "first stable of 26.09", tags: []string{"beta-26.09"}, args: []string{"stable", "beta/26.09"}, want: "stable-26.09"},
		{name: "a patch of 26.08", tags: []string{"stable-26.08"}, args: []string{"stable", "beta/26.08"}, want: "stable-26.08.1"},
		{name: "a hotfix of patch 08", tags: []string{"stable-26.09.08"}, args: []string{"prerelease", "hotfix/x"}, want: "hotfix-26.09.9"},
		{name: "26.09 outranks 26.8", tags: []string{"stable-26.8", "beta-26.8"}, args: []string{"prerelease", "beta/26.09"}, want: "beta-26.09"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := releaseTag(t, tc.tags, tc.args...)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// macOS ships bash 3.2 as /bin/bash; a repository with no tags yet must work
// under it too.
func TestReleaseTag_NoTagsUnderEveryBash(t *testing.T) {
	shells := []string{"/bin/bash"}
	if bash, err := exec.LookPath("bash"); err == nil {
		shells = append(shells, bash)
	}
	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			if _, err := os.Stat(shell); err != nil || runtime.GOOS == "windows" {
				t.Skip("no " + shell)
			}
			got, err := releaseTagWith(t, shell, nil, "stable", "beta/26.5")
			require.NoError(t, err)
			assert.Equal(t, "stable-26.5", got)
		})
	}
}
