package fletcher_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/picker"
)

func withMatch(
	pick picker.Pick,
	match picker.Match,
	nameMatch bool,
) picker.Pick {
	pick.Match = match
	pick.NameMatch = nameMatch
	return pick
}

func TestFletcher_Confidence(t *testing.T) {
	testCases := []struct {
		name     string
		picks    map[domain.OS]picker.Pick
		want     fletcher.Confidence
		warnings []string
	}{
		{
			name: "every pick exact and named is high",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64:   exactPick("tool-linux-amd64.tar.gz", picker.FormatArchive),
				domain.OSWindowsAMD64: exactPick("tool-windows-amd64.zip", picker.FormatArchive),
				domain.OSWindowsARM64: exactPick("tool-windows-arm64", picker.FormatBinary),
			},
			want: fletcher.ConfidenceHigh,
		},
		{
			name: "assumed arch is medium",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: withMatch(exactPick("tool-linux.tar.gz", picker.FormatArchive), picker.MatchAssumed, true),
			},
			want:     fletcher.ConfidenceMedium,
			warnings: []string{fletcher.WarningAssumedArch},
		},
		{
			name: "emulated arch is medium",
			picks: map[domain.OS]picker.Pick{
				domain.OSDarwinARM64: withMatch(exactPick("tool-darwin-amd64.zip", picker.FormatArchive), picker.MatchEmulated, true),
			},
			want:     fletcher.ConfidenceMedium,
			warnings: []string{fletcher.WarningEmulated},
		},
		{
			name: "windows bare exe is medium",
			picks: map[domain.OS]picker.Pick{
				domain.OSWindowsAMD64: exactPick("Tool-x64.EXE", picker.FormatBinary),
			},
			want:     fletcher.ConfidenceMedium,
			warnings: []string{fletcher.WarningWindowsExeUnverified},
		},
		{
			name: "linux binary named exe is not a windows exe",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: exactPick("tool.exe", picker.FormatBinary),
			},
			want: fletcher.ConfidenceHigh,
		},
		{
			name: "name mismatch is low",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: withMatch(exactPick("other-linux.tar.gz", picker.FormatArchive), picker.MatchExact, false),
			},
			want:     fletcher.ConfidenceLow,
			warnings: []string{fletcher.WarningNameMismatch},
		},
		{
			name: "low wins over medium and warnings are ordered and unique",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64:   withMatch(exactPick("other.tar.gz", picker.FormatArchive), picker.MatchAssumed, false),
				domain.OSLinuxARM64:   withMatch(exactPick("other-arm.tar.gz", picker.FormatArchive), picker.MatchExact, false),
				domain.OSDarwinARM64:  withMatch(exactPick("tool-mac.zip", picker.FormatArchive), picker.MatchEmulated, true),
				domain.OSWindowsAMD64: withMatch(exactPick("tool.exe", picker.FormatBinary), picker.MatchAssumed, true),
			},
			want: fletcher.ConfidenceLow,
			warnings: []string{
				fletcher.WarningAssumedArch,
				fletcher.WarningEmulated,
				fletcher.WarningWindowsExeUnverified,
				fletcher.WarningNameMismatch,
			},
		},
		{
			name: "unknown match raises nothing",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: withMatch(exactPick("tool.tar.gz", picker.FormatArchive), picker.Match("other"), true),
			},
			want: fletcher.ConfidenceHigh,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := fletcher.New(lookupOf(&stubForge{assets: realAssets()}), &stubPicker{picks: tc.picks})

			draft, err := f.Probe(context.Background(), testNS, testTag, fletcher.Hint{})
			require.NoError(t, err)

			arrow := parse(t, draft.Manifest)
			assert.Equal(t, tc.want, draft.Report.Confidence)
			assert.Equal(t, tc.warnings, draft.Report.Warnings)
			assert.Equal(t, string(tc.want), arrow.Generator.Confidence)
			assert.Equal(t, tc.warnings, arrow.Generator.Warnings)
		})
	}
}

func TestConfidence_Values(t *testing.T) {
	assert.Equal(t, fletcher.Confidence("high"), fletcher.ConfidenceHigh)
	assert.Equal(t, fletcher.Confidence("medium"), fletcher.ConfidenceMedium)
	assert.Equal(t, fletcher.Confidence("low"), fletcher.ConfidenceLow)
	assert.Equal(t, "assumed_arch", fletcher.WarningAssumedArch)
	assert.Equal(t, "emulated", fletcher.WarningEmulated)
	assert.Equal(t, "name_mismatch", fletcher.WarningNameMismatch)
	assert.Equal(t, "windows_exe_unverified", fletcher.WarningWindowsExeUnverified)
}
