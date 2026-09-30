package picker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func labelled(
	name string,
	label string,
) domain.ReleaseAsset {
	a := asset(name)
	a.Label = label
	return a
}

func githubDesktopRelease() []domain.ReleaseAsset {
	return []domain.ReleaseAsset{
		labelled("GitHub.Desktop-3.6.6-checksums.txt", "GitHub Desktop 3.6.6 checksums"),
		labelled("GitHub.Desktop-arm64.zip", "GitHub Desktop 3.6.6 macOS arm64"),
		labelled("GitHub.Desktop-x64.zip", "GitHub Desktop 3.6.6 macOS x64"),
		labelled("GitHubDesktop-3.6.6-x64-delta.nupkg", "GitHub Desktop 3.6.6 Windows x64 Delta Nupkg"),
		labelled("GitHubDesktop-3.6.6-x64-full.nupkg", "GitHub Desktop 3.6.6 Windows x64 Full Nupkg"),
		labelled("GitHubDesktopSetup-x64.exe", "GitHub Desktop 3.6.6 Windows x64 EXE Installer"),
		labelled("GitHubDesktopSetup-x64.msi", "GitHub Desktop 3.6.6 Windows x64 MSI Installer"),
	}
}

func TestPicker_Pick_LabelCarriesTheOSAndTheFileNameTheFormat(t *testing.T) {
	testCases := []struct {
		name      string
		os        domain.OS
		wantAsset string
		wantFmt   Format
	}{
		{name: "darwin arm64", os: domain.OSDarwinARM64, wantAsset: "GitHub.Desktop-arm64.zip", wantFmt: FormatArchive},
		{name: "darwin amd64", os: domain.OSDarwinAMD64, wantAsset: "GitHub.Desktop-x64.zip", wantFmt: FormatArchive},
	}
	p := New()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := p.Pick("desktop/desktop", githubDesktopRelease(), tc.os)

			require.True(t, ok)
			assert.Equal(t, tc.wantAsset, got.Asset.Name)
			assert.Equal(t, tc.wantFmt, got.Format)
			assert.Equal(t, MatchExact, got.Match)
		})
	}
}

func TestPicker_Pick_LabelDoesNotChangeWhatAFileNameAloneDecides(t *testing.T) {
	testCases := []struct {
		name   string
		assets []domain.ReleaseAsset
		os     domain.OS
	}{
		{name: "a label naming another OS does not move a tagged file", os: domain.OSLinuxAMD64, assets: []domain.ReleaseAsset{labelled("tool-darwin-arm64.zip", "Tool for macOS")}},
		{name: "a label cannot make an unrecognised file an asset of the target", os: domain.OSDarwinARM64, assets: []domain.ReleaseAsset{labelled("tool-linux-amd64.tar.gz", "Tool macOS arm64")}},
	}
	p := New()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := p.Pick("u/tool", tc.assets, tc.os)

			assert.False(t, ok)
		})
	}
}

func TestPicker_Pick_InstallerWordGluedToTheProductIsNotPartOfIt(t *testing.T) {
	testCases := []struct {
		name        string
		asset       string
		wantProduct string
	}{
		{name: "setup suffix", asset: "GitHubDesktopSetup-x64.msi", wantProduct: "githubdesktop"},
		{name: "installer suffix", asset: "ToolInstaller-x64.msi", wantProduct: "tool"},
	}
	p, _ := New().(*picker)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := p.classify(asset(tc.asset))

			require.True(t, ok)
			assert.Equal(t, tc.wantProduct, got.product)
		})
	}
}
