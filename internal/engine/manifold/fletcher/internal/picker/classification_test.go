package picker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestPicker_Classify_Kept(t *testing.T) {
	testCases := []struct {
		name           string
		asset          string
		wantFamily     string
		wantArch       string
		wantFormat     Format
		wantMusl       bool
		wantExe        bool
		wantStem       string
		wantProduct    string
		wantCompanions []string
	}{
		{name: "linux token", asset: "tool-linux-amd64.tar.gz", wantFamily: familyLinux, wantArch: archAMD64, wantFormat: FormatArchive, wantStem: "tool-linux-amd64", wantProduct: "tool"},
		{name: "rust triple musl", asset: "bat-v0.26.1-x86_64-unknown-linux-musl.tar.gz", wantFamily: familyLinux, wantArch: archAMD64, wantFormat: FormatArchive, wantMusl: true, wantStem: "bat-v0.26.1-x86_64-unknown-linux-musl", wantProduct: "bat"},
		{name: "musl alone implies linux", asset: "tool-musl-x86_64.tar.xz", wantFamily: familyLinux, wantArch: archAMD64, wantFormat: FormatArchive, wantMusl: true, wantStem: "tool-musl-x86_64", wantProduct: "tool"},
		{name: "manylinux", asset: "tool-manylinux_2_17-aarch64.tar.gz", wantFamily: familyLinux, wantArch: archARM64, wantFormat: FormatArchive, wantStem: "tool-manylinux_2_17-aarch64", wantProduct: "tool"},
		{name: "darwin token", asset: "tool_Darwin_arm64.tar.gz", wantFamily: familyDarwin, wantArch: archARM64, wantFormat: FormatArchive, wantStem: "tool_darwin_arm64", wantProduct: "tool"},
		{name: "apple triple", asset: "rg-aarch64-apple-darwin.tar.gz", wantFamily: familyDarwin, wantArch: archARM64, wantFormat: FormatArchive, wantStem: "rg-aarch64-apple-darwin", wantProduct: "rg"},
		{name: "macos token", asset: "app-macOS-universal.zip", wantFamily: familyDarwin, wantArch: archUniversal, wantFormat: FormatArchive, wantStem: "app-macos-universal", wantProduct: "app"},
		{name: "osx token", asset: "tool.osx.x64.tgz", wantFamily: familyDarwin, wantArch: archAMD64, wantFormat: FormatArchive, wantStem: "tool.osx.x64", wantProduct: "tool"},
		{name: "windows token", asset: "tool_Windows_x86_64.zip", wantFamily: familyWindows, wantArch: archAMD64, wantFormat: FormatArchive, wantStem: "tool_windows_x86_64", wantProduct: "tool"},
		{name: "msvc triple", asset: "typst-aarch64-pc-windows-msvc.zip", wantFamily: familyWindows, wantArch: archARM64, wantFormat: FormatArchive, wantStem: "typst-aarch64-pc-windows-msvc", wantProduct: "typst"},
		{name: "win32 with x64 is amd64", asset: "claude-win32-x64.zip", wantFamily: familyWindows, wantArch: archAMD64, wantFormat: FormatArchive, wantStem: "claude-win32-x64", wantProduct: "claude"},
		{name: "win32 alone is 32-bit", asset: "tool-win32.zip", wantFamily: familyWindows, wantArch: archOther, wantFormat: FormatArchive, wantStem: "tool-win32", wantProduct: "tool"},
		{name: "win32 with x86 is other", asset: "tool-win32-x86.zip", wantFamily: familyWindows, wantArch: archOther, wantFormat: FormatArchive, wantStem: "tool-win32-x86", wantProduct: "tool"},
		{name: "dmg implies darwin", asset: "App-1.0.dmg", wantFamily: familyDarwin, wantArch: archNone, wantFormat: FormatDMG, wantStem: "app-1.0", wantProduct: "app"},
		{name: "exe implies windows", asset: "tool-amd64.exe", wantFamily: familyWindows, wantArch: archAMD64, wantFormat: FormatBinary, wantExe: true, wantStem: "tool-amd64", wantProduct: "tool"},
		{name: "appimage implies linux", asset: "Crowbar_nightly_aarch64.AppImage", wantFamily: familyLinux, wantArch: archARM64, wantFormat: FormatAppImage, wantStem: "crowbar_nightly_aarch64", wantProduct: "crowbar"},
		{name: "extension overrides name token", asset: "tool-linux-x64.exe", wantFamily: familyWindows, wantArch: archAMD64, wantFormat: FormatBinary, wantExe: true, wantStem: "tool-linux-x64", wantProduct: "tool"},
		{name: "bare binary", asset: "gitea-1.27.3-darwin-10.12-arm64", wantFamily: familyDarwin, wantArch: archARM64, wantFormat: FormatBinary, wantStem: "gitea-1.27.3-darwin-10.12-arm64", wantProduct: "gitea"},
		{name: "single file gzip", asset: "zed-remote-server-macos-aarch64.gz", wantFamily: familyDarwin, wantArch: archARM64, wantFormat: FormatArchive, wantStem: "zed-remote-server-macos-aarch64", wantProduct: "zedremoteserver", wantCompanions: []string{"remote", "server"}},
		{name: "zstd tar", asset: "ollama-linux-arm64.tar.zst", wantFamily: familyLinux, wantArch: archARM64, wantFormat: FormatArchive, wantStem: "ollama-linux-arm64", wantProduct: "ollama"},
		{name: "tbz2 longest suffix", asset: "tool-linux-amd64.tbz2", wantFamily: familyLinux, wantArch: archAMD64, wantFormat: FormatArchive, wantStem: "tool-linux-amd64", wantProduct: "tool"},
		{name: "universal2", asset: "tool-macos-universal2.tar.gz", wantFamily: familyDarwin, wantArch: archUniversal, wantFormat: FormatArchive, wantStem: "tool-macos-universal2", wantProduct: "tool"},
		{name: "64bit", asset: "tool_linux_64bit.tar.gz", wantFamily: familyLinux, wantArch: archAMD64, wantFormat: FormatArchive, wantStem: "tool_linux_64bit", wantProduct: "tool"},
		{name: "arm32", asset: "Godot_v3.6.3-stable_linux.arm32.zip", wantFamily: familyLinux, wantArch: archOther, wantFormat: FormatArchive, wantStem: "godot_v3.6.3-stable_linux.arm32", wantProduct: "godot"},
		{name: "other arch", asset: "tool-linux-armv7.tar.gz", wantFamily: familyLinux, wantArch: archOther, wantFormat: FormatArchive, wantStem: "tool-linux-armv7", wantProduct: "tool"},
		{name: "debugger is not debug", asset: "react-native-debugger-linux-x64.zip", wantFamily: familyLinux, wantArch: archAMD64, wantFormat: FormatArchive, wantStem: "react-native-debugger-linux-x64", wantProduct: "reactnativedebugger"},
		{name: "channel tokens removed from product", asset: "tool-v2.0.0-rc1-nightly-canary-linux-amd64.tar.gz", wantFamily: familyLinux, wantArch: archAMD64, wantFormat: FormatArchive, wantStem: "tool-v2.0.0-rc1-nightly-canary-linux-amd64", wantProduct: "tool"},
		{name: "product free asset", asset: "linux-x64.tar.gz", wantFamily: familyLinux, wantArch: archAMD64, wantFormat: FormatArchive, wantStem: "linux-x64", wantProduct: ""},
		{name: "version token is not a product", asset: "v2rayN-linux-arm64.zip", wantFamily: familyLinux, wantArch: archARM64, wantFormat: FormatArchive, wantStem: "v2rayn-linux-arm64", wantProduct: "v2rayn"},
		{name: "companion tokens", asset: "ragflow-cli-v0.27.2-linux-amd64", wantFamily: familyLinux, wantArch: archAMD64, wantFormat: FormatBinary, wantStem: "ragflow-cli-v0.27.2-linux-amd64", wantProduct: "ragflowcli", wantCompanions: []string{"cli"}},
		{name: "product normalised", asset: "Cherry-Studio-2.1.3-linux-x64.AppImage", wantFamily: familyLinux, wantArch: archAMD64, wantFormat: FormatAppImage, wantStem: "cherry-studio-2.1.3-linux-x64", wantProduct: "cherrystudio"},
	}

	p := New().(*picker)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := p.classify(asset(tc.asset))

			require.True(t, ok)
			assert.Equal(t, tc.asset, got.asset.Name)
			assert.Equal(t, tc.wantFamily, got.family)
			assert.Equal(t, tc.wantArch, got.arch)
			assert.Equal(t, tc.wantFormat, got.format)
			assert.Equal(t, tc.wantMusl, got.musl)
			assert.Equal(t, tc.wantExe, got.exe)
			assert.Equal(t, tc.wantStem, got.stem)
			assert.Equal(t, tc.wantProduct, got.product)
			assert.Equal(t, tc.wantCompanions, got.companions)
		})
	}
}

func TestPicker_Classify_Excluded(t *testing.T) {
	testCases := []struct {
		name  string
		asset domain.ReleaseAsset
	}{
		{name: "no digest", asset: domain.ReleaseAsset{Name: "tool-linux-amd64.tar.gz"}},
		{name: "checksum file", asset: asset("tool-linux-amd64.tar.gz.sha256")},
		{name: "signature", asset: asset("tool-linux-amd64.tar.gz.sig")},
		{name: "sbom", asset: asset("tool-linux-amd64.spdx")},
		{name: "json", asset: asset("latest-linux.json")},
		{name: "wheel", asset: asset("tool-1.0-py3-none-manylinux_2_17_x86_64.whl")},
		{name: "jar", asset: asset("tool-linux.jar")},
		{name: "aar", asset: asset("tool-linux.aar")},
		{name: "apk", asset: asset("tool-linux-arm64.apk")},
		{name: "shared library", asset: asset("libtool-linux-x64.so")},
		{name: "font", asset: asset("Ubuntu-linux.ttf")},
		{name: "pacman", asset: asset("tool-1.0-x86_64.pkg.tar.zst")},
		{name: "tauri updater bundle", asset: asset("App_aarch64.app.tar.gz")},
		{name: "checksums token", asset: asset("tool_checksums_linux.tar.gz")},
		{name: "sha256sums", asset: asset("SHA256SUMS")},
		{name: "source archive", asset: asset("tool-src-linux.tar.gz")},
		{name: "debug token", asset: asset("tool-linux-amd64-debug.tar.gz")},
		{name: "dsym token", asset: asset("OBS-Studio-32.2.2-macOS-Apple-dSYMs.tar.xz")},
		{name: "pdb token", asset: asset("tool-windows-x64-pdb.zip")},
		{name: "symbols token", asset: asset("tool.symbols.linux.tar.gz")},
		{name: "docs token", asset: asset("tool-docs-linux.zip")},
		{name: "sdk token", asset: asset("tool-sdk-linux-x64.zip")},
		{name: "msi installer", asset: asset("tool-1.0-x64.msi")},
		{name: "msix installer", asset: asset("tool-windows-x64.msix")},
		{name: "pkg installer", asset: asset("Tool-1.0.pkg")},
		{name: "deb installer", asset: asset("tool_1.0_amd64.deb")},
		{name: "rpm installer", asset: asset("tool-1.0.x86_64.rpm")},
		{name: "snap installer", asset: asset("tool_1.0_amd64.snap")},
		{name: "flatpak installer", asset: asset("tool-linux.flatpak")},
		{name: "setup exe", asset: asset("Tool-Setup-1.0.exe")},
		{name: "installer exe", asset: asset("tool-windows-x64-installer.exe")},
		{name: "desktop exe", asset: asset("AnythingLLMDesktop.exe")},
		{name: "desktop token exe", asset: asset("Unsloth-Desktop-Windows.exe")},
		{name: "nsis exe", asset: asset("app-nsis-x64.exe")},
		{name: "squirrel exe", asset: asset("App-Squirrel-1.0.exe")},
		{name: "7z unsupported", asset: asset("tool-windows-x64.7z")},
		{name: "unknown extension", asset: asset("tool-linux-amd64.foo")},
		{name: "no os", asset: asset("tool-amd64.tar.gz")},
		{name: "conflicting os", asset: asset("tool-x86_64-pc-windows-gnu.zip")},
		{name: "linux and mac", asset: asset("tool-linux-mac.tar.gz")},
	}

	p := New().(*picker)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := p.classify(tc.asset)

			assert.False(t, ok)
		})
	}
}

func TestClassification_Named_Product(t *testing.T) {
	assert.True(t, classification{product: "tool"}.named())
	assert.False(t, classification{}.named())
}

func TestPicker_Classify_ExoticArches(t *testing.T) {
	testCases := []struct {
		name  string
		asset string
	}{
		{name: "armv5", asset: "tool_linux_armv5.tar.gz"},
		{name: "armv5te", asset: "tool_linux_armv5te.tar.gz"},
		{name: "arm5", asset: "tool-linux-arm5.tar.gz"},
		{name: "powerpc64", asset: "tool-linux-powerpc64.tar.gz"},
		{name: "powerpc64le", asset: "tool-linux-powerpc64le.tar.gz"},
		{name: "i586", asset: "tool-linux-i586.tar.gz"},
		{name: "sparc64", asset: "tool-linux-sparc64.tar.gz"},
	}

	p := New().(*picker)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := p.classify(asset(tc.asset))

			require.True(t, ok)
			assert.Equal(t, archOther, got.arch)
		})
	}
}

func TestClassification_GUIInstaller_Exe(t *testing.T) {
	testCases := []struct {
		name  string
		class classification
		want  bool
	}{
		{name: "plain exe", class: classification{exe: true}, want: true},
		{name: "portable exe", class: classification{exe: true, portable: true}},
		{name: "not exe", class: classification{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.class.guiInstaller())
		})
	}
}

func TestClassification_Supported_Arch(t *testing.T) {
	assert.True(t, classification{arch: archAMD64}.supported())
	assert.True(t, classification{arch: archNone}.supported())
	assert.False(t, classification{arch: archOther}.supported())
}

func TestPicker_Classify_ExoticArchTokensLeaveProduct(t *testing.T) {
	testCases := []struct {
		name  string
		asset string
	}{
		{name: "armv7", asset: "foo-server-linux-armv7.tar.gz"},
		{name: "i686", asset: "foo-server-i686-unknown-linux-gnu.tar.gz"},
		{name: "riscv64", asset: "foo-server_linux_riscv64.tar.gz"},
		{name: "s390x", asset: "foo-server-linux-s390x.tar.gz"},
		{name: "ppc64le", asset: "foo-server-linux-ppc64le.tar.gz"},
		{name: "arm", asset: "foo-server-linux-arm.tar.gz"},
	}

	p := New().(*picker)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := p.classify(asset(tc.asset))

			require.True(t, ok)
			assert.Equal(t, "fooserver", got.product)
		})
	}
}

func TestPicker_Identify_Repos(t *testing.T) {
	testCases := []struct {
		name string
		repo string
		want repoIdentity
	}{
		{name: "owner and repo", repo: "Byron/dua-cli", want: repoIdentity{name: "dua-cli", product: "duacli", tokens: []string{"dua", "cli"}}},
		{name: "no owner", repo: "Tool", want: repoIdentity{name: "tool", product: "tool", tokens: []string{"tool"}}},
		{name: "symbols only", repo: "u/--", want: repoIdentity{name: "--", product: "", tokens: []string{"", ""}}},
	}

	p := New().(*picker)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, p.identify(tc.repo))
		})
	}
}

func TestRepoIdentity_Accepts_Candidates(t *testing.T) {
	testCases := []struct {
		name  string
		id    repoIdentity
		class classification
		want  bool
	}{
		{name: "plain product", id: repoIdentity{tokens: []string{"tool"}}, class: classification{product: "tool"}, want: true},
		{name: "foreign companion", id: repoIdentity{tokens: []string{"tool"}}, class: classification{product: "toolcli", companions: []string{"cli"}}},
		{name: "companion in repo name", id: repoIdentity{tokens: []string{"tool", "cli"}}, class: classification{product: "toolcli", companions: []string{"cli"}}, want: true},
		{name: "one foreign among many", id: repoIdentity{tokens: []string{"tool", "cli"}}, class: classification{product: "toolcliserver", companions: []string{"cli", "server"}}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.id.accepts(tc.class))
		})
	}
}

func TestRepoIdentity_Owns_Products(t *testing.T) {
	testCases := []struct {
		name    string
		id      repoIdentity
		product string
		want    bool
	}{
		{name: "equal", id: repoIdentity{product: "crowbar"}, product: "crowbar", want: true},
		{name: "suffixed", id: repoIdentity{product: "crowbar"}, product: "crowbarapi"},
		{name: "empty identity", id: repoIdentity{}, product: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.id.owns(classification{product: tc.product}))
		})
	}
}

func TestRepoIdentity_Extends_Products(t *testing.T) {
	testCases := []struct {
		name    string
		id      repoIdentity
		product string
		want    bool
	}{
		{name: "equal", id: repoIdentity{product: "tool"}, product: "tool", want: true},
		{name: "suffixed", id: repoIdentity{product: "tool"}, product: "toolkit", want: true},
		{name: "shorter", id: repoIdentity{product: "toolcli"}, product: "tool"},
		{name: "unrelated", id: repoIdentity{product: "pake"}, product: "grok"},
		{name: "empty identity", id: repoIdentity{}, product: "tool"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.id.extends(classification{product: tc.product}))
		})
	}
}

func TestRepoIdentity_MentionedIn_Names(t *testing.T) {
	testCases := []struct {
		name  string
		id    repoIdentity
		asset string
		want  bool
	}{
		{name: "prefix", id: repoIdentity{name: "zap"}, asset: "Zap-linux.tar.gz", want: true},
		{name: "inside", id: repoIdentity{name: "zap"}, asset: "go-zap-linux.tar.gz", want: true},
		{name: "absent", id: repoIdentity{name: "zap"}, asset: "other-linux.tar.gz"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.id.mentionedIn(classification{asset: asset(tc.asset)}))
		})
	}
}

func TestRepoIdentity_Extras_Counts(t *testing.T) {
	testCases := []struct {
		name    string
		id      repoIdentity
		product string
		want    int
	}{
		{name: "equal", id: repoIdentity{product: "tool"}, product: "tool", want: 0},
		{name: "suffix", id: repoIdentity{product: "tool"}, product: "toolserver", want: 6},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.id.extras(classification{product: tc.product}))
		})
	}
}
