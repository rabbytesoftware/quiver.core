package picker

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

type corpusEntry struct {
	Repo   string        `json:"repo"`
	Tag    string        `json:"tag"`
	Assets []corpusAsset `json:"assets"`
}

type corpusAsset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

type label struct {
	Repo   string    `json:"repo"`
	OS     domain.OS `json:"os"`
	Want   string    `json:"want"`
	Reject string    `json:"reject"`
	Strict bool      `json:"strict"`
}

func asset(name string) hosts.Asset {
	return hosts.Asset{Name: name, URL: "https://example.com/" + name, Size: 1, Digest: "sha256:" + name}
}

func assets(names ...string) []hosts.Asset {
	out := make([]hosts.Asset, 0, len(names))
	for _, n := range names {
		out = append(out, asset(n))
	}
	return out
}

func loadJSON(t *testing.T, name string, into any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, into))
}

func loadCorpus(t *testing.T) map[string][]hosts.Asset {
	t.Helper()
	var entries []corpusEntry
	loadJSON(t, "corpus.json", &entries)
	out := make(map[string][]hosts.Asset, len(entries))
	for _, e := range entries {
		list := make([]hosts.Asset, 0, len(e.Assets))
		for _, a := range e.Assets {
			list = append(list, hosts.Asset{Name: a.Name, Size: a.Size, Digest: a.Digest})
		}
		out[e.Repo] = list
	}
	return out
}

func TestPicker_Pick_Rules(t *testing.T) {
	testCases := []struct {
		name      string
		repo      string
		assets    []hosts.Asset
		os        domain.OS
		wantOK    bool
		wantAsset string
		wantFmt   Format
		wantMatch Match
		wantName  bool
	}{
		{
			name:      "exact arch archive",
			repo:      "u/tool",
			assets:    assets("tool-linux-amd64.tar.gz", "tool-linux-arm64.tar.gz", "tool-darwin-arm64.tar.gz"),
			os:        domain.OSLinuxARM64,
			wantOK:    true,
			wantAsset: "tool-linux-arm64.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "bare binary",
			repo:      "u/tool",
			assets:    assets("tool_windows_amd64.exe", "tool_linux_amd64"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "tool_linux_amd64",
			wantFmt:   FormatBinary,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "portable exe",
			repo:      "u/tool",
			assets:    assets("tool_windows_amd64.exe", "tool_linux_amd64"),
			os:        domain.OSWindowsAMD64,
			wantOK:    true,
			wantAsset: "tool_windows_amd64.exe",
			wantFmt:   FormatBinary,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "appimage implies linux",
			repo:      "u/app",
			assets:    assets("App-1.0-x86_64.AppImage"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "App-1.0-x86_64.AppImage",
			wantFmt:   FormatAppImage,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "dmg allowed",
			repo:      "u/app",
			assets:    assets("App-1.0-arm64.dmg", "App-1.0-x64.dmg"),
			os:        domain.OSDarwinARM64,
			wantOK:    true,
			wantAsset: "App-1.0-arm64.dmg",
			wantFmt:   FormatDMG,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:   "installer only target refused",
			repo:   "u/app",
			assets: assets("App-1.0-x64.msi", "App-Setup-1.0.exe", "app_1.0_amd64.deb", "app-1.0.x86_64.rpm", "App-1.0.pkg"),
			os:     domain.OSWindowsAMD64,
		},
		{
			name:   "installer only linux refused",
			repo:   "u/app",
			assets: assets("app_1.0_amd64.deb", "app-1.0.x86_64.rpm", "app_1.0_amd64.snap", "app.flatpak"),
			os:     domain.OSLinuxAMD64,
		},
		{
			name:   "installer only darwin pkg refused",
			repo:   "u/app",
			assets: assets("App-1.0.pkg"),
			os:     domain.OSDarwinARM64,
		},
		{
			name:      "archive beats dmg",
			repo:      "u/app",
			assets:    assets("app-macos-universal.zip", "App.dmg"),
			os:        domain.OSDarwinARM64,
			wantOK:    true,
			wantAsset: "app-macos-universal.zip",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:   "digest-less assets excluded",
			repo:   "u/tool",
			assets: []hosts.Asset{{Name: "tool-linux-amd64.tar.gz"}},
			os:     domain.OSLinuxAMD64,
		},
		{
			name:      "digest-less sibling ignored",
			repo:      "u/tool",
			assets:    append([]hosts.Asset{{Name: "tool-linux-amd64-full.tar.gz"}}, asset("tool-linux-amd64.tar.gz")),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "tool-linux-amd64.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "universal counts as exact on darwin",
			repo:      "u/app",
			assets:    assets("app-macos-universal.tar.gz"),
			os:        domain.OSDarwinAMD64,
			wantOK:    true,
			wantAsset: "app-macos-universal.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:   "universal is not exact on linux",
			repo:   "u/app",
			assets: assets("app-linux-universal.tar.gz"),
			os:     domain.OSLinuxAMD64,
		},
		{
			name:      "archless assumed amd64",
			repo:      "u/tool",
			assets:    assets("tool-linux.tar.gz"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "tool-linux.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchAssumed,
			wantName:  true,
		},
		{
			name:   "never assume arm64 on linux",
			repo:   "u/tool",
			assets: assets("tool-linux.tar.gz", "tool-linux-amd64.tar.gz"),
			os:     domain.OSLinuxARM64,
		},
		{
			name:      "archless assumed on darwin arm64",
			repo:      "u/app",
			assets:    assets("App.dmg", "app-macos-x64.zip"),
			os:        domain.OSDarwinARM64,
			wantOK:    true,
			wantAsset: "App.dmg",
			wantFmt:   FormatDMG,
			wantMatch: MatchAssumed,
			wantName:  true,
		},
		{
			name:      "emulated darwin arm64 uses amd64",
			repo:      "u/tool",
			assets:    assets("tool-darwin-amd64.tar.gz", "tool-linux-arm64.tar.gz"),
			os:        domain.OSDarwinARM64,
			wantOK:    true,
			wantAsset: "tool-darwin-amd64.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchEmulated,
			wantName:  true,
		},
		{
			name:      "emulated windows arm64 uses amd64",
			repo:      "u/tool",
			assets:    assets("tool-windows-x86_64.zip"),
			os:        domain.OSWindowsARM64,
			wantOK:    true,
			wantAsset: "tool-windows-x86_64.zip",
			wantFmt:   FormatArchive,
			wantMatch: MatchEmulated,
			wantName:  true,
		},
		{
			name:   "no emulation on linux arm64",
			repo:   "u/tool",
			assets: assets("tool-linux-amd64.tar.gz"),
			os:     domain.OSLinuxARM64,
		},
		{
			name:   "no emulation for amd64 targets",
			repo:   "u/tool",
			assets: assets("tool-darwin-arm64.tar.gz"),
			os:     domain.OSDarwinAMD64,
		},
		{
			name:   "32-bit only refused",
			repo:   "u/tool",
			assets: assets("tool-windows-386.zip", "tool-linux-armv7.tar.gz"),
			os:     domain.OSWindowsAMD64,
		},
		{
			name:      "musl preferred over gnu",
			repo:      "u/tool",
			assets:    assets("tool-x86_64-unknown-linux-gnu.tar.gz", "tool-x86_64-unknown-linux-musl.tar.gz"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "tool-x86_64-unknown-linux-musl.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "exact product beats suffixed product",
			repo:      "char2cs/crowbar",
			assets:    assets("crowbar-api-linux-amd64", "Crowbar_nightly_amd64.AppImage", "Crowbar_nightly_amd64.deb"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "Crowbar_nightly_amd64.AppImage",
			wantFmt:   FormatAppImage,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "exact product beats better format",
			repo:      "char2cs/crowbar",
			assets:    assets("crowbar-api-darwin-arm64", "Crowbar_nightly_universal.dmg"),
			os:        domain.OSDarwinARM64,
			wantOK:    true,
			wantAsset: "Crowbar_nightly_universal.dmg",
			wantFmt:   FormatDMG,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "channel tokens neutral",
			repo:      "u/tool",
			assets:    assets("tool-extra-linux-amd64.tar.gz", "toolkit-nightly-linux-amd64.tar.gz"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "toolkit-nightly-linux-amd64.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
		},
		{
			name:      "fewest extras among related products",
			repo:      "u/tool",
			assets:    assets("tool-fullest-linux-amd64.tar.gz", "tool-full-linux-amd64.tar.gz"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "tool-full-linux-amd64.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
		},
		{
			name:   "unrelated products are not tiebroken",
			repo:   "u/tool-cli",
			assets: assets("tool-linux-amd64.tar.gz", "tool-extra-linux-amd64.tar.gz"),
			os:     domain.OSLinuxAMD64,
		},
		{
			name:      "companion only kept without name match",
			repo:      "infiniflow/ragflow",
			assets:    assets("ragflow-cli-v0.27.2-linux-amd64", "install.sh", "SHA256SUMS"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "ragflow-cli-v0.27.2-linux-amd64",
			wantFmt:   FormatBinary,
			wantMatch: MatchExact,
		},
		{
			name:      "language binding companion kept without name match",
			repo:      "xtekky/gpt4free",
			assets:    assets("g4f-go-v8.5.8-linux-arm64.zip", "g4f-8.5.8.tar.gz"),
			os:        domain.OSLinuxARM64,
			wantOK:    true,
			wantAsset: "g4f-go-v8.5.8-linux-arm64.zip",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
		},
		{
			name:      "companion dropped when an alternative exists in another arch",
			repo:      "zed-industries/zed",
			assets:    assets("zed-remote-server-macos-aarch64.gz", "Zed-x86_64.dmg"),
			os:        domain.OSDarwinARM64,
			wantOK:    true,
			wantAsset: "Zed-x86_64.dmg",
			wantFmt:   FormatDMG,
			wantMatch: MatchEmulated,
			wantName:  true,
		},
		{
			name:      "companion name match cleared even when product equals repo",
			repo:      "u/ragflowcli",
			assets:    assets("ragflow-cli-linux-amd64"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "ragflow-cli-linux-amd64",
			wantFmt:   FormatBinary,
			wantMatch: MatchExact,
		},
		{
			name:   "exe in a gui release is an installer",
			repo:   "diegosouzapw/OmniRoute",
			assets: assets("OmniRoute.3.8.50.exe", "OmniRoute-3.8.50.dmg", "OmniRoute-3.8.50.AppImage"),
			os:     domain.OSWindowsAMD64,
		},
		{
			name:   "exe in an appimage-only gui release is an installer",
			repo:   "u/app",
			assets: assets("App-1.0-x64.exe", "App-1.0-x86_64.AppImage"),
			os:     domain.OSWindowsAMD64,
		},
		{
			name:   "digest-less gui package still marks the release",
			repo:   "u/app",
			assets: append(assets("App-1.0-x64.exe"), hosts.Asset{Name: "App-1.0.DMG"}),
			os:     domain.OSWindowsAMD64,
		},
		{
			name:      "gui release windows archive still picked",
			repo:      "u/app",
			assets:    assets("App-1.0-x64.exe", "App-1.0-windows-x64.zip", "App-1.0.dmg"),
			os:        domain.OSWindowsAMD64,
			wantOK:    true,
			wantAsset: "App-1.0-windows-x64.zip",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:   "desktop exe is an installer",
			repo:   "unslothai/unsloth",
			assets: assets("Unsloth-Desktop-Windows.exe", "Unsloth-Desktop-Linux.tar.gz"),
			os:     domain.OSWindowsAMD64,
		},
		{
			name:      "portable exe in a gui release stays a binary",
			repo:      "CherryHQ/cherry-studio",
			assets:    assets("Cherry-Studio-2.1.3-win-x64-portable.exe", "Cherry-Studio-2.1.3-arm64.dmg"),
			os:        domain.OSWindowsAMD64,
			wantOK:    true,
			wantAsset: "Cherry-Studio-2.1.3-win-x64-portable.exe",
			wantFmt:   FormatBinary,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:   "companion refused when the product ships only as an installer here",
			repo:   "zed-industries/zed",
			assets: assets("Zed-x86_64.exe", "zed-remote-server-windows-x86_64.zip", "Zed-aarch64.dmg"),
			os:     domain.OSWindowsAMD64,
		},
		{
			name:      "exe in a cli release stays a binary",
			repo:      "FiloSottile/mkcert",
			assets:    assets("mkcert-v1.4.4-windows-amd64.exe", "mkcert-v1.4.4-linux-amd64", "mkcert-v1.4.4-darwin-arm64.tar.gz"),
			os:        domain.OSWindowsAMD64,
			wantOK:    true,
			wantAsset: "mkcert-v1.4.4-windows-amd64.exe",
			wantFmt:   FormatBinary,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "companion-only release with armv7 still picks",
			repo:      "u/foo",
			assets:    assets("foo-server-linux-amd64", "foo-server-linux-arm64", "foo-server-linux-armv7", "foo-server-darwin-arm64", "foo-server-windows-amd64.zip"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "foo-server-linux-amd64",
			wantFmt:   FormatBinary,
			wantMatch: MatchExact,
		},
		{
			name:      "companion-only release with armv7 still picks windows",
			repo:      "u/foo",
			assets:    assets("foo-server-linux-amd64", "foo-server-linux-arm64", "foo-server-linux-armv7", "foo-server-darwin-arm64", "foo-server-windows-amd64.zip"),
			os:        domain.OSWindowsAMD64,
			wantOK:    true,
			wantAsset: "foo-server-windows-amd64.zip",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
		},
		{
			name:      "companion-only release with i686 still picks",
			repo:      "u/foo",
			assets:    assets("foo-server-x86_64-unknown-linux-gnu.tar.gz", "foo-server-i686-unknown-linux-gnu.tar.gz", "foo-server-x86_64-pc-windows-msvc.zip"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "foo-server-x86_64-unknown-linux-gnu.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
		},
		{
			name:      "companion-only release with riscv64 still picks",
			repo:      "u/foo",
			assets:    assets("foo-server_linux_amd64.tar.gz", "foo-server_linux_riscv64.tar.gz", "foo-server_windows_amd64.zip"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "foo-server_linux_amd64.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
		},
		{
			name:   "different companion-only products refused",
			repo:   "u/foo",
			assets: assets("foo-server-linux-amd64.tar.gz", "foo-cli-linux-amd64.tar.gz"),
			os:     domain.OSLinuxAMD64,
		},
		{
			name:      "same stem prefers archive over exe",
			repo:      "localsend/localsend",
			assets:    assets("LocalSend-1.18.2-windows-x86-64.exe", "LocalSend-1.18.2-windows-x86-64.zip"),
			os:        domain.OSWindowsAMD64,
			wantOK:    true,
			wantAsset: "LocalSend-1.18.2-windows-x86-64.zip",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "same stem prefers archive over appimage",
			repo:      "neovim/nvim",
			assets:    assets("nvim-linux-arm64.appimage", "nvim-linux-arm64.tar.gz"),
			os:        domain.OSLinuxARM64,
			wantOK:    true,
			wantAsset: "nvim-linux-arm64.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "same stem prefers appimage over binary",
			repo:      "u/tool",
			assets:    assets("tool-linux-amd64", "tool-linux-amd64.AppImage"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "tool-linux-amd64.AppImage",
			wantFmt:   FormatAppImage,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "same stem prefers binary over exe",
			repo:      "u/tool",
			assets:    assets("tool-windows-amd64.exe", "tool-windows-amd64"),
			os:        domain.OSWindowsAMD64,
			wantOK:    true,
			wantAsset: "tool-windows-amd64",
			wantFmt:   FormatBinary,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "same stem lexical order breaks format ties",
			repo:      "ollama/ollama",
			assets:    assets("ollama-darwin.tgz", "Ollama-darwin.zip"),
			os:        domain.OSDarwinARM64,
			wantOK:    true,
			wantAsset: "Ollama-darwin.zip",
			wantFmt:   FormatArchive,
			wantMatch: MatchAssumed,
			wantName:  true,
		},
		{
			name: "identical digests are one candidate",
			repo: "jqlang/jq",
			assets: []hosts.Asset{
				{Name: "jq-linux64", URL: "https://example.com/jq-linux64", Digest: "sha256:same"},
				{Name: "jq-linux-amd64", URL: "https://example.com/jq-linux-amd64", Digest: "sha256:same"},
			},
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "jq-linux-amd64",
			wantFmt:   FormatBinary,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:   "different digests with different stems stay ambiguous",
			repo:   "jqlang/jq",
			assets: assets("jq-linux64", "jq-linux-amd64"),
			os:     domain.OSLinuxAMD64,
		},
		{
			name:      "native arch beats universal on darwin",
			repo:      "u/app",
			assets:    assets("app-macos-universal.zip", "app-macos-arm64.zip"),
			os:        domain.OSDarwinARM64,
			wantOK:    true,
			wantAsset: "app-macos-arm64.zip",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:   "exotic arches are never assumed",
			repo:   "u/tool",
			assets: assets("tool-linux-powerpc64.tar.gz", "tool-linux-powerpc64le.tar.gz", "tool-linux-armv5.tar.gz", "tool-linux-arm5.tar.gz", "tool-linux-i586.tar.gz", "tool-linux-sparc64.tar.gz"),
			os:     domain.OSLinuxAMD64,
		},
		{
			name:      "companion allowed when repo carries it",
			repo:      "u/tool-cli",
			assets:    assets("tool-cli-linux-amd64.tar.gz", "tool-server-linux-amd64.tar.gz"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "tool-cli-linux-amd64.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:   "product-less asset refused",
			repo:   "wadey/node-microtime",
			assets: assets("linux-x64.tar.gz"),
			os:     domain.OSLinuxAMD64,
		},
		{
			name:   "os word as whole product refused",
			repo:   "ryanoasis/nerd-fonts",
			assets: assets("Ubuntu.tar.xz", "Ubuntu.zip", "Hack.zip"),
			os:     domain.OSLinuxAMD64,
		},
		{
			name:      "channel token keeps exact product",
			repo:      "u/tool",
			assets:    assets("tool-api-linux-amd64", "tool_beta_rc2_linux_amd64"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "tool_beta_rc2_linux_amd64",
			wantFmt:   FormatBinary,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "companion variant excluded",
			repo:      "u/tool-cli",
			assets:    assets("tool-server-linux-amd64.tar.gz", "tool-linux-amd64.tar.gz"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "tool-linux-amd64.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
		},
		{
			name:      "repo name mention preferred",
			repo:      "u/zap",
			assets:    assets("zap-linux-amd64-extras.tar.gz", "other-linux-amd64.tar.gz"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "zap-linux-amd64-extras.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
		},
		{
			name:   "ambiguous variants refused",
			repo:   "u/pake",
			assets: assets("Grok.dmg", "ChatGPT.dmg"),
			os:     domain.OSDarwinARM64,
		},
		{
			name:      "same stem different archive is not ambiguous",
			repo:      "u/tool",
			assets:    assets("tool-windows-amd64.zip", "tool-windows-amd64.tar.gz"),
			os:        domain.OSWindowsAMD64,
			wantOK:    true,
			wantAsset: "tool-windows-amd64.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:   "no assets",
			repo:   "u/tool",
			assets: nil,
			os:     domain.OSLinuxAMD64,
		},
		{
			name:   "unknown os refused",
			repo:   "u/tool",
			assets: assets("tool-linux-amd64.tar.gz", "tool-freebsd-amd64.tar.gz", "tool.tar.gz"),
			os:     domain.OS("freebsd/amd64"),
		},
		{
			name:      "repo without owner",
			repo:      "tool",
			assets:    assets("tool-linux-amd64.tar.gz"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "tool-linux-amd64.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
			wantName:  true,
		},
		{
			name:      "symbol only repo never name matches",
			repo:      "u/--",
			assets:    assets("tool-linux-amd64.tar.gz"),
			os:        domain.OSLinuxAMD64,
			wantOK:    true,
			wantAsset: "tool-linux-amd64.tar.gz",
			wantFmt:   FormatArchive,
			wantMatch: MatchExact,
		},
	}

	p := New()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := p.Pick(tc.repo, tc.assets, tc.os)

			require.Equal(t, tc.wantOK, ok)
			if !tc.wantOK {
				assert.Equal(t, Pick{}, got)
				return
			}
			assert.Equal(t, tc.wantAsset, got.Asset.Name)
			assert.Equal(t, "https://example.com/"+tc.wantAsset, got.Asset.URL)
			assert.Equal(t, tc.wantFmt, got.Format)
			assert.Equal(t, tc.wantMatch, got.Match)
			assert.Equal(t, tc.wantName, got.NameMatch)
		})
	}
}

func TestPicker_Pick_CrowbarNightly(t *testing.T) {
	corpus := loadCorpus(t)
	crowbar, ok := corpus["char2cs/crowbar"]
	require.True(t, ok)

	testCases := []struct {
		name string
		os   domain.OS
		want string
	}{
		{name: "darwin arm64", os: domain.OSDarwinARM64, want: "Crowbar_nightly_universal.dmg"},
		{name: "darwin amd64", os: domain.OSDarwinAMD64, want: "Crowbar_nightly_universal.dmg"},
		{name: "linux amd64", os: domain.OSLinuxAMD64, want: "Crowbar_nightly_amd64.AppImage"},
		{name: "linux arm64", os: domain.OSLinuxARM64, want: "Crowbar_nightly_aarch64.AppImage"},
	}

	p := New()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := p.Pick("char2cs/crowbar", crowbar, tc.os)

			require.True(t, ok)
			assert.Equal(t, tc.want, got.Asset.Name)
			assert.NotContains(t, got.Asset.Name, "crowbar-api")
			assert.True(t, got.NameMatch)
		})
	}

	for _, target := range domain.AllOS() {
		got, _ := p.Pick("char2cs/crowbar", crowbar, target)
		assert.NotContains(t, got.Asset.Name, "crowbar-api", target)
	}
}

func TestPicker_Pick_Labels(t *testing.T) {
	corpus := loadCorpus(t)
	var labels []label
	loadJSON(t, "labels.json", &labels)
	require.NotEmpty(t, labels)

	p := New()
	for _, l := range labels {
		repoAssets, ok := corpus[l.Repo]
		require.True(t, ok, l.Repo)
		got, picked := p.Pick(l.Repo, repoAssets, l.OS)
		subject := l.Repo + " " + string(l.OS)

		switch {
		case l.Reject != "" && l.Strict:
			t.Run("strict reject is never picked/"+subject, func(t *testing.T) {
				assert.NotEqual(t, l.Reject, got.Asset.Name)
			})
		case l.Reject != "":
			t.Run("reject is never picked with name match/"+subject, func(t *testing.T) {
				assert.False(t, got.Asset.Name == l.Reject && got.NameMatch)
			})
		case l.Want == "none":
			t.Run("want none yields no pick/"+subject, func(t *testing.T) {
				assert.False(t, picked, got.Asset.Name)
			})
		default:
			t.Run("want asset is picked exactly/"+subject, func(t *testing.T) {
				assert.Equal(t, l.Want, got.Asset.Name)
			})
		}
	}
}

func sortedRepos(
	corpus map[string][]hosts.Asset,
) []string {
	repos := make([]string, 0, len(corpus))
	for repo := range corpus {
		repos = append(repos, repo)
	}
	sort.Strings(repos)
	return repos
}

func assertOrderIndependent(
	t *testing.T,
	p Picker,
	rng *rand.Rand,
	repo string,
	repoAssets []hosts.Asset,
	target domain.OS,
) {
	t.Helper()
	want, wantOK := p.Pick(repo, repoAssets, target)
	for range 3 {
		shuffled := slices.Clone(repoAssets)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		got, ok := p.Pick(repo, shuffled, target)
		require.Equal(t, wantOK, ok, "%s %s", repo, target)
		require.Equal(t, want, got, "%s %s", repo, target)
	}
}

func TestPicker_Pick_OrderIndependent(t *testing.T) {
	corpus := loadCorpus(t)
	rng := rand.New(rand.NewPCG(10, 26))
	p := New()

	for _, repo := range sortedRepos(corpus) {
		for _, target := range domain.AllOS() {
			assertOrderIndependent(t, p, rng, repo, corpus[repo], target)
		}
	}
}

func TestPicker_Pick_Baseline(t *testing.T) {
	corpus := loadCorpus(t)
	var baseline map[domain.OS]int
	loadJSON(t, "baseline.json", &baseline)
	require.Len(t, baseline, len(domain.AllOS()))

	p := New()
	counts := make(map[domain.OS]int, len(baseline))
	for repo, repoAssets := range corpus {
		for _, target := range domain.AllOS() {
			if _, ok := p.Pick(repo, repoAssets, target); ok {
				counts[target]++
			}
		}
	}

	for _, target := range domain.AllOS() {
		want, ok := baseline[target]
		require.True(t, ok, target)
		assert.GreaterOrEqual(t, counts[target], want, target)
	}
}
