package picker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestPicker_Pick_DottedProductNames(t *testing.T) {
	testCases := []struct {
		name      string
		repo      string
		asset     string
		os        domain.OS
		wantFmt   Format
		wantMatch Match
		wantName  bool
	}{
		{name: "zip with a dotted product", repo: "desktop/desktop", asset: "GitHub.Desktop-macos-arm64.zip", os: domain.OSDarwinARM64, wantFmt: FormatArchive, wantMatch: MatchExact},
		{name: "x64 zip with a dotted product", repo: "desktop/desktop", asset: "GitHub.Desktop-macos-x64.zip", os: domain.OSDarwinAMD64, wantFmt: FormatArchive, wantMatch: MatchExact},
		{name: "tar.gz with a dotted product", repo: "u/Some.App", asset: "Some.App-darwin-arm64.tar.gz", os: domain.OSDarwinARM64, wantFmt: FormatArchive, wantMatch: MatchExact, wantName: true},
		{name: "tar.xz with a dotted product", repo: "u/Some.App", asset: "Some.App-linux-amd64.tar.xz", os: domain.OSLinuxAMD64, wantFmt: FormatArchive, wantMatch: MatchExact, wantName: true},
		{name: "tgz with several dots", repo: "u/a.b.c", asset: "a.b.c-linux-arm64.tgz", os: domain.OSLinuxARM64, wantFmt: FormatArchive, wantMatch: MatchExact, wantName: true},
		{name: "windows zip with a dotted product", repo: "u/Some.App", asset: "Some.App-windows-amd64.zip", os: domain.OSWindowsAMD64, wantFmt: FormatArchive, wantMatch: MatchExact, wantName: true},
		{name: "versioned dotted archive", repo: "u/Some.App", asset: "Some.App-1.2.3-darwin-arm64.zip", os: domain.OSDarwinARM64, wantFmt: FormatArchive, wantMatch: MatchExact, wantName: true},
		{name: "bare binary with a dotted product", repo: "u/Some.App", asset: "Some.App-linux-amd64", os: domain.OSLinuxAMD64, wantFmt: FormatBinary, wantMatch: MatchExact, wantName: true},
	}
	p := New()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := p.Pick(tc.repo, assets(tc.asset), tc.os)

			require.True(t, ok)
			assert.Equal(t, tc.asset, got.Asset.Name)
			assert.Equal(t, tc.wantFmt, got.Format)
			assert.Equal(t, tc.wantMatch, got.Match)
			assert.Equal(t, tc.wantName, got.NameMatch)
		})
	}
}
