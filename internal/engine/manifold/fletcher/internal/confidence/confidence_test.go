package confidence_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/confidence"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
)

const digestA = "sha256:00000000000000000000000000000000000000000000000000000000000000aa"

func exactPick(
	name string,
	format picker.Format,
) picker.Pick {
	return picker.Pick{
		Asset:     domain.ReleaseAsset{Name: name, URL: "https://example.test/" + name, Digest: digestA},
		Format:    format,
		Match:     picker.MatchExact,
		NameMatch: true,
	}
}

func withMatch(
	pick picker.Pick,
	match picker.Match,
	nameMatch bool,
) picker.Pick {
	pick.Match = match
	pick.NameMatch = nameMatch
	return pick
}

func product(
	name string,
	format picker.Format,
) picker.Pick {
	pick := exactPick(name+"-asset", format)
	pick.NameMatch = false
	pick.Product = name
	pick.Accepted = true
	return pick
}

func TestAssess_CompanionProductIsNotConsistent(t *testing.T) {
	companion := product("gadget", picker.FormatArchive)
	companion.Accepted = false
	picks := map[domain.OS]picker.Pick{domain.OSLinuxAMD64: companion}

	got := confidence.Assess(picks, false)

	assert.Equal(t, confidence.ConfidenceLow, got.Level)
	assert.False(t, got.KeepsAny())
}

func TestAssess_RollingTagCapsConfidenceAtMedium(t *testing.T) {
	picks := map[domain.OS]picker.Pick{
		domain.OSLinuxAMD64: exactPick("tool-linux.tar.gz", picker.FormatArchive),
	}

	got := confidence.Assess(picks, true)

	assert.Equal(t, confidence.ConfidenceMedium, got.Level)
	assert.Equal(t, []string{confidence.WarningUnpinnedRollingTag}, got.Warnings)
	assert.Len(t, got.Picks, 1)
}

func TestAssess(t *testing.T) {
	testCases := []struct {
		name     string
		picks    map[domain.OS]picker.Pick
		want     confidence.Confidence
		warnings []string
		kept     int
	}{
		{
			name: "every pick exact and named is high",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64:   exactPick("tool-linux-amd64.tar.gz", picker.FormatArchive),
				domain.OSWindowsAMD64: exactPick("tool-windows-amd64.zip", picker.FormatArchive),
				domain.OSWindowsARM64: exactPick("tool-windows-arm64", picker.FormatBinary),
			},
			want: confidence.ConfidenceHigh,
			kept: 3,
		},
		{
			name: "assumed arch is medium",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: withMatch(exactPick("tool-linux.tar.gz", picker.FormatArchive), picker.MatchAssumed, true),
			},
			want:     confidence.ConfidenceMedium,
			warnings: []string{confidence.WarningAssumedArch},
			kept:     1,
		},
		{
			name: "emulated arch is medium",
			picks: map[domain.OS]picker.Pick{
				domain.OSDarwinARM64: withMatch(exactPick("tool-darwin-amd64.zip", picker.FormatArchive), picker.MatchEmulated, true),
			},
			want:     confidence.ConfidenceMedium,
			warnings: []string{confidence.WarningEmulated},
			kept:     1,
		},
		{
			name: "windows bare exe is medium",
			picks: map[domain.OS]picker.Pick{
				domain.OSWindowsAMD64: exactPick("Tool-x64.EXE", picker.FormatBinary),
			},
			want:     confidence.ConfidenceMedium,
			warnings: []string{confidence.WarningWindowsExeUnverified},
			kept:     1,
		},
		{
			name: "linux binary named exe is not a windows exe",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: exactPick("tool.exe", picker.FormatBinary),
			},
			want: confidence.ConfidenceHigh,
			kept: 1,
		},
		{
			name: "medium warnings are ordered and unique",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64:   withMatch(exactPick("tool.tar.gz", picker.FormatArchive), picker.MatchAssumed, true),
				domain.OSLinuxARM64:   withMatch(exactPick("tool-arm.tar.gz", picker.FormatArchive), picker.MatchAssumed, true),
				domain.OSDarwinARM64:  withMatch(exactPick("tool-mac.zip", picker.FormatArchive), picker.MatchEmulated, true),
				domain.OSWindowsAMD64: withMatch(exactPick("tool.exe", picker.FormatBinary), picker.MatchAssumed, true),
			},
			want: confidence.ConfidenceMedium,
			warnings: []string{
				confidence.WarningAssumedArch,
				confidence.WarningEmulated,
				confidence.WarningWindowsExeUnverified,
			},
			kept: 4,
		},
		{
			name: "unknown match raises nothing",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: withMatch(exactPick("tool.tar.gz", picker.FormatArchive), picker.Match("other"), true),
			},
			want: confidence.ConfidenceHigh,
			kept: 1,
		},
		{
			name: "a lone name mismatch with no product is low",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: withMatch(exactPick("other-linux.tar.gz", picker.FormatArchive), picker.MatchExact, false),
			},
			want:     confidence.ConfidenceLow,
			warnings: []string{confidence.WarningNameMismatch},
		},
		{
			name: "an inconsistent platform is dropped and the rest stay medium",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64:  withMatch(exactPick("other.tar.gz", picker.FormatArchive), picker.MatchAssumed, false),
				domain.OSDarwinARM64: withMatch(exactPick("tool-mac.zip", picker.FormatArchive), picker.MatchEmulated, true),
			},
			want: confidence.ConfidenceMedium,
			warnings: []string{
				confidence.WarningEmulated,
				confidence.WarningNameMismatch,
			},
			kept: 1,
		},
		{
			name: "one product across every platform survives without the repo name",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64:  product("zen", picker.FormatAppImage),
				domain.OSDarwinARM64: product("zen", picker.FormatDMG),
			},
			want:     confidence.ConfidenceMedium,
			warnings: []string{confidence.WarningNameMismatch},
			kept:     2,
		},
		{
			name: "the repo named product outvotes a stray one and the stray platform is dropped",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64:   exactPick("T3-Code.AppImage", picker.FormatAppImage),
				domain.OSDarwinARM64:  exactPick("T3-Code.dmg", picker.FormatDMG),
				domain.OSWindowsAMD64: product("t3", picker.FormatArchive),
			},
			want:     confidence.ConfidenceMedium,
			warnings: []string{confidence.WarningNameMismatch},
			kept:     2,
		},
		{
			name: "unrelated products with no repo match tie and nothing survives",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64:  product("alpha", picker.FormatArchive),
				domain.OSDarwinARM64: product("beta", picker.FormatArchive),
			},
			want:     confidence.ConfidenceLow,
			warnings: []string{confidence.WarningNameMismatch},
		},
		{
			name: "the majority product wins when no pick matches the repo name",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64:   product("zen", picker.FormatArchive),
				domain.OSLinuxARM64:   product("zen", picker.FormatArchive),
				domain.OSWindowsAMD64: product("other", picker.FormatArchive),
			},
			want:     confidence.ConfidenceMedium,
			warnings: []string{confidence.WarningNameMismatch},
			kept:     2,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := confidence.Assess(tc.picks, false)

			assert.Equal(t, tc.want, got.Level)
			assert.Equal(t, tc.warnings, got.Warnings)
			assert.Equal(t, tc.kept, len(got.Picks))
			assert.Equal(t, tc.kept > 0, got.KeepsAny())
		})
	}
}
