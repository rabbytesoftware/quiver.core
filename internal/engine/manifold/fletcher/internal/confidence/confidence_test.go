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

func TestAssess(t *testing.T) {
	testCases := []struct {
		name     string
		picks    map[domain.OS]picker.Pick
		want     confidence.Confidence
		warnings []string
	}{
		{
			name: "every pick exact and named is high",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64:   exactPick("tool-linux-amd64.tar.gz", picker.FormatArchive),
				domain.OSWindowsAMD64: exactPick("tool-windows-amd64.zip", picker.FormatArchive),
				domain.OSWindowsARM64: exactPick("tool-windows-arm64", picker.FormatBinary),
			},
			want: confidence.ConfidenceHigh,
		},
		{
			name: "assumed arch is medium",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: withMatch(exactPick("tool-linux.tar.gz", picker.FormatArchive), picker.MatchAssumed, true),
			},
			want:     confidence.ConfidenceMedium,
			warnings: []string{confidence.WarningAssumedArch},
		},
		{
			name: "emulated arch is medium",
			picks: map[domain.OS]picker.Pick{
				domain.OSDarwinARM64: withMatch(exactPick("tool-darwin-amd64.zip", picker.FormatArchive), picker.MatchEmulated, true),
			},
			want:     confidence.ConfidenceMedium,
			warnings: []string{confidence.WarningEmulated},
		},
		{
			name: "windows bare exe is medium",
			picks: map[domain.OS]picker.Pick{
				domain.OSWindowsAMD64: exactPick("Tool-x64.EXE", picker.FormatBinary),
			},
			want:     confidence.ConfidenceMedium,
			warnings: []string{confidence.WarningWindowsExeUnverified},
		},
		{
			name: "linux binary named exe is not a windows exe",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: exactPick("tool.exe", picker.FormatBinary),
			},
			want: confidence.ConfidenceHigh,
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
		},
		{
			name: "unknown match raises nothing",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: withMatch(exactPick("tool.tar.gz", picker.FormatArchive), picker.Match("other"), true),
			},
			want: confidence.ConfidenceHigh,
		},
		{
			name: "name mismatch is low",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: withMatch(exactPick("other-linux.tar.gz", picker.FormatArchive), picker.MatchExact, false),
			},
			want:     confidence.ConfidenceLow,
			warnings: []string{confidence.WarningNameMismatch},
		},
		{
			name: "one mismatched pick among medium ones is low",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64:  withMatch(exactPick("other.tar.gz", picker.FormatArchive), picker.MatchAssumed, false),
				domain.OSDarwinARM64: withMatch(exactPick("tool-mac.zip", picker.FormatArchive), picker.MatchEmulated, true),
			},
			want: confidence.ConfidenceLow,
			warnings: []string{
				confidence.WarningAssumedArch,
				confidence.WarningEmulated,
				confidence.WarningNameMismatch,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, warnings := confidence.Assess(tc.picks)

			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.warnings, warnings)
		})
	}
}
