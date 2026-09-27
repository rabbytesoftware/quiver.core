package forge_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/forge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/picker"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

const digest = "sha256:0000000000000000000000000000000000000000000000000000000000000001"

func pickOf(
	name string,
	format picker.Format,
) picker.Pick {
	return picker.Pick{
		Asset: hosts.Asset{
			Name:   name,
			URL:    "https://github.com/acme/tool/releases/download/v1/" + name,
			Digest: digest,
		},
		Format:    format,
		Match:     picker.MatchExact,
		NameMatch: true,
	}
}

func baseInput() forge.Input {
	return forge.Input{
		Repo:        "tool",
		Name:        "tool",
		Description: "A tool that does things.",
		URL:         "https://github.com/acme/tool",
		Media:       domain.ArrowMedia{Icon: "https://example.test/icon.png"},
		Readme:      "Tool does things.\n\nIt is fast.",
		Generator:   domain.ArrowGenerator{Name: "fletcher/1", Confidence: "medium", Warnings: []string{"assumed_arch"}},
		Picks: map[domain.OS]picker.Pick{
			domain.OSLinuxAMD64:   pickOf("tool-linux-x86_64.tar.gz", picker.FormatArchive),
			domain.OSLinuxARM64:   pickOf("tool-aarch64.AppImage", picker.FormatAppImage),
			domain.OSDarwinAMD64:  pickOf("tool-darwin-amd64.zip", picker.FormatArchive),
			domain.OSDarwinARM64:  pickOf("tool-arm64.dmg", picker.FormatDMG),
			domain.OSWindowsAMD64: pickOf("tool-windows-amd64.exe", picker.FormatBinary),
			domain.OSWindowsARM64: pickOf("tool-windows-arm64.zip", picker.FormatArchive),
		},
	}
}

func parse(
	t *testing.T,
	data []byte,
) *domain.Arrow {
	t.Helper()
	arrow, err := manifold.NewWithResolvers(nil, nil, nil).ParseArrow(data)
	require.NoError(t, err, string(data))
	return arrow
}

func yamlBlock(
	t *testing.T,
	data []byte,
) string {
	t.Helper()
	text := string(data)
	start := strings.Index(text, "\n```arrow\n")
	require.GreaterOrEqual(t, start, 0)
	body := text[start+len("\n```arrow\n"):]
	require.True(t, strings.HasSuffix(body, "```\n"))
	return strings.TrimSuffix(body, "```\n")
}

func fetchStep(
	t *testing.T,
	s step.Step,
) step.FetchStep {
	t.Helper()
	fetch, ok := s.(step.FetchStep)
	require.True(t, ok, "%T is not a fetch step", s)
	return fetch
}

func portableStep(
	t *testing.T,
	s step.Step,
) step.PortableStep {
	t.Helper()
	portable, ok := s.(step.PortableStep)
	require.True(t, ok, "%T is not a portable step", s)
	return portable
}

func TestRender_ParsesWithMetadataAndGenerator(t *testing.T) {
	data, err := forge.Render(baseInput())
	require.NoError(t, err)

	arrow := parse(t, data)

	assert.Equal(t, "tool", arrow.Name)
	assert.Equal(t, "A tool that does things.", arrow.Description)
	assert.Equal(t, "https://github.com/acme/tool", arrow.URL)
	assert.Equal(t, domain.ArrowMedia{Icon: "https://example.test/icon.png"}, arrow.Media)
	assert.Equal(t, &domain.ArrowGenerator{Name: "fletcher/1", Confidence: "medium", Warnings: []string{"assumed_arch"}}, arrow.Generator)
	assert.Equal(t, domain.ArrowOriginInferred, arrow.Origin())
	assert.Equal(t, "Tool does things.\n\nIt is fast.", arrow.Readme)
	assert.Empty(t, arrow.License)
	assert.Empty(t, arrow.Tags)
	assert.Len(t, arrow.Targets, 6)
	assert.True(t, strings.HasPrefix(string(data), "Tool does things.\n\nIt is fast.\n\n```arrow\n"))
}

func TestRender_ReparseKeepsGenerator(t *testing.T) {
	data, err := forge.Render(baseInput())
	require.NoError(t, err)

	first := parse(t, data)
	second := parse(t, data)

	assert.Equal(t, first.Generator, second.Generator)
	assert.Equal(t, first.Origin(), second.Origin())
	assert.Equal(t, domain.ArrowOriginInferred, second.Origin())
}

func TestRender_Targets(t *testing.T) {
	testCases := []struct {
		name         string
		os           domain.OS
		fetchTo      string
		portableFrom string
		cli          []domain.ExposeEntry
		desktop      []domain.ExposeEntry
	}{
		{
			name:         "linux archive fetches, installs and exposes a cli",
			os:           domain.OSLinuxAMD64,
			fetchTo:      "${INSTALL_PATH}/tool-linux-x86_64.tar.gz",
			portableFrom: "${INSTALL_PATH}/tool-linux-x86_64.tar.gz",
			cli:          []domain.ExposeEntry{{Name: "tool", Path: domain.ExposeAuto}},
		},
		{
			name:         "appimage fetches, installs and exposes a desktop entry",
			os:           domain.OSLinuxARM64,
			fetchTo:      "${INSTALL_PATH}/tool-aarch64.AppImage",
			portableFrom: "${INSTALL_PATH}/tool-aarch64.AppImage",
			desktop:      []domain.ExposeEntry{{Name: "tool", Path: domain.ExposeAuto}},
		},
		{
			name:         "darwin archive exposes cli and desktop",
			os:           domain.OSDarwinAMD64,
			fetchTo:      "${INSTALL_PATH}/tool-darwin-amd64.zip",
			portableFrom: "${INSTALL_PATH}/tool-darwin-amd64.zip",
			cli:          []domain.ExposeEntry{{Name: "tool", Path: domain.ExposeAuto}},
			desktop:      []domain.ExposeEntry{{Name: "tool", Path: domain.ExposeAuto}},
		},
		{
			name:         "dmg fetches, installs and exposes a desktop entry",
			os:           domain.OSDarwinARM64,
			fetchTo:      "${INSTALL_PATH}/tool-arm64.dmg",
			portableFrom: "${INSTALL_PATH}/tool-arm64.dmg",
			desktop:      []domain.ExposeEntry{{Name: "tool", Path: domain.ExposeAuto}},
		},
		{
			name:    "windows binary lands as the repo name with exe",
			os:      domain.OSWindowsAMD64,
			fetchTo: "${INSTALL_PATH}/tool.exe",
			cli:     []domain.ExposeEntry{{Name: "tool", Path: "${INSTALL_PATH}/tool.exe"}},
		},
		{
			name:         "windows archive exposes only a cli",
			os:           domain.OSWindowsARM64,
			fetchTo:      "${INSTALL_PATH}/tool-windows-arm64.zip",
			portableFrom: "${INSTALL_PATH}/tool-windows-arm64.zip",
			cli:          []domain.ExposeEntry{{Name: "tool", Path: domain.ExposeAuto}},
		},
	}
	in := baseInput()
	data, err := forge.Render(in)
	require.NoError(t, err)
	arrow := parse(t, data)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target, ok := arrow.Targets[tc.os]
			require.True(t, ok)
			wantSteps := 1
			if tc.portableFrom != "" {
				wantSteps = 2
			}

			require.Len(t, target.Lifecycle.Install, wantSteps)
			assert.Empty(t, target.Lifecycle.Update)
			assert.Empty(t, target.Lifecycle.Uninstall)
			fetch := fetchStep(t, target.Lifecycle.Install[0])
			assert.Equal(t, in.Picks[tc.os].Asset.URL, fetch.URL.Default)
			assert.Equal(t, tc.fetchTo, fetch.To.Default)
			assert.Equal(t, digest, fetch.Checksum.Default)
			assert.Equal(t, "15m", fetch.Timeout.Default)
			assert.Equal(t, "Download "+in.Picks[tc.os].Asset.Name, fetch.Title())
			assert.Equal(t, tc.cli, target.Expose.CLI)
			assert.Equal(t, tc.desktop, target.Expose.Desktop)
			if tc.portableFrom == "" {
				return
			}
			portable := portableStep(t, target.Lifecycle.Install[1])
			assert.Equal(t, tc.portableFrom, portable.From.Default)
			assert.Equal(t, "${INSTALL_PATH}", portable.To.Default)
			assert.Equal(t, "Install "+in.Picks[tc.os].Asset.Name, portable.Title())
		})
	}
}

func TestRender_GUIArchiveExposesDesktop(t *testing.T) {
	testCases := []struct {
		name string
		os   domain.OS
	}{
		{name: "linux archive with GUI", os: domain.OSLinuxAMD64},
		{name: "windows archive with GUI", os: domain.OSWindowsARM64},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			in := baseInput()
			pick := in.Picks[tc.os]
			pick.GUI = true
			in.Picks = map[domain.OS]picker.Pick{tc.os: pick}
			data, err := forge.Render(in)
			require.NoError(t, err)

			arrow := parse(t, data)
			target := arrow.Targets[tc.os]
			want := []domain.ExposeEntry{{Name: "tool", Path: domain.ExposeAuto}}
			assert.Equal(t, want, target.Expose.CLI)
			assert.Equal(t, want, target.Expose.Desktop)
		})
	}
}

func TestRender_OnlyPickedTargets(t *testing.T) {
	in := baseInput()
	in.Picks = map[domain.OS]picker.Pick{
		domain.OSLinuxAMD64: pickOf("tool-linux", picker.FormatBinary),
	}
	data, err := forge.Render(in)
	require.NoError(t, err)

	arrow := parse(t, data)

	require.Len(t, arrow.Targets, 1)
	fetch := fetchStep(t, arrow.Targets[domain.OSLinuxAMD64].Lifecycle.Install[0])
	assert.Equal(t, "${INSTALL_PATH}/tool", fetch.To.Default)
	assert.Equal(t,
		[]domain.ExposeEntry{{Name: "tool", Path: "${INSTALL_PATH}/tool"}},
		arrow.Targets[domain.OSLinuxAMD64].Expose.CLI,
	)
	assert.Contains(t, yamlBlock(t, data), "\n  linux/amd64:\n")
}

func TestRender_SanitisesExposeAndBinaryName(t *testing.T) {
	in := baseInput()
	in.Repo = "my tool!"
	in.Name = "my tool!"
	in.Picks = map[domain.OS]picker.Pick{
		domain.OSWindowsAMD64: pickOf("tool.exe", picker.FormatBinary),
	}
	data, err := forge.Render(in)
	require.NoError(t, err)

	arrow := parse(t, data)

	target := arrow.Targets[domain.OSWindowsAMD64]
	assert.Equal(t, "my tool!", arrow.Name)
	assert.Equal(t, []domain.ExposeEntry{{Name: "my-tool-", Path: "${INSTALL_PATH}/my-tool-.exe"}}, target.Expose.CLI)
	assert.Equal(t, "${INSTALL_PATH}/my-tool-.exe", fetchStep(t, target.Lifecycle.Install[0]).To.Default)
}

func TestRender_ProseFallback(t *testing.T) {
	testCases := []struct {
		name        string
		readme      string
		description string
		want        string
	}{
		{name: "readme wins", readme: "Prose.", description: "Desc.", want: "Prose."},
		{name: "blank readme falls back to description", readme: "  \n", description: "Desc.", want: "Desc."},
		{name: "multi-line description collapses", readme: "", description: "Desc\n  more.", want: "Desc more."},
		{name: "no description falls back to name", readme: "", description: "", want: "tool"},
		{name: "fence-like description is neutralised", readme: "", description: "```arrow", want: "~~~arrow"},
		{name: "fence lines in readme are neutralised", readme: "Intro.\n```arrow\nschema: evil\n```", description: "", want: "Intro.\n~~~arrow\nschema: evil\n~~~"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			in := baseInput()
			in.Readme = tc.readme
			in.Description = tc.description
			data, err := forge.Render(in)
			require.NoError(t, err)

			arrow := parse(t, data)

			assert.Equal(t, tc.want, arrow.Readme)
		})
	}
}

func TestRender_DropsUnusablePicks(t *testing.T) {
	testCases := []struct {
		name  string
		asset hosts.Asset
	}{
		{name: "empty digest", asset: hosts.Asset{Name: "tool.tar.gz", URL: "https://example.test/tool.tar.gz"}},
		{name: "dollar in file name", asset: hosts.Asset{Name: "x", URL: "https://example.test/$%7BHOME%7D", Digest: digest}},
		{name: "parent directory", asset: hosts.Asset{Name: "x", URL: "https://example.test/a/..", Digest: digest}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			in := baseInput()
			in.Picks = map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: pickOf("tool-linux.tar.gz", picker.FormatArchive),
				domain.OSLinuxARM64: {Asset: tc.asset, Format: picker.FormatArchive, Match: picker.MatchExact, NameMatch: true},
			}
			data, err := forge.Render(in)
			require.NoError(t, err)

			arrow := parse(t, data)

			assert.Contains(t, arrow.Targets, domain.OSLinuxAMD64)
			assert.NotContains(t, arrow.Targets, domain.OSLinuxARM64)
		})
	}
}

func TestRender_FileNameComesFromURL(t *testing.T) {
	in := baseInput()
	in.Picks = map[domain.OS]picker.Pick{
		domain.OSLinuxAMD64: {
			Asset:     hosts.Asset{Name: "../../${HOME}/x\nevil", URL: "https://example.test/dl/tool%2Bv1.tar.gz?x=1", Digest: digest},
			Format:    picker.FormatArchive,
			Match:     picker.MatchExact,
			NameMatch: true,
		},
	}
	data, err := forge.Render(in)
	require.NoError(t, err)

	target := parse(t, data).Targets[domain.OSLinuxAMD64]

	fetch := fetchStep(t, target.Lifecycle.Install[0])
	portable := portableStep(t, target.Lifecycle.Install[1])
	assert.Equal(t, "${INSTALL_PATH}/tool+v1.tar.gz", fetch.To.Default)
	assert.Equal(t, "Download tool+v1.tar.gz", fetch.Title())
	assert.Equal(t, "${INSTALL_PATH}/tool+v1.tar.gz", portable.From.Default)
	assert.Equal(t, "Install tool+v1.tar.gz", portable.Title())
	assert.NotContains(t, string(data), "${HOME}")
}

func TestRender_EmptyMediaAndWarningsAreOmitted(t *testing.T) {
	in := baseInput()
	in.Media = domain.ArrowMedia{}
	in.Generator = domain.ArrowGenerator{Name: "fletcher/1", Confidence: "high"}
	data, err := forge.Render(in)
	require.NoError(t, err)

	block := yamlBlock(t, data)
	arrow := parse(t, data)

	assert.NotContains(t, block, "media:")
	assert.NotContains(t, block, "warnings:")
	assert.NotContains(t, block, "uninstall:")
	assert.Nil(t, arrow.Generator.Warnings)
	assert.Equal(t, domain.ArrowMedia{}, arrow.Media)
}

func TestRender_YAMLHasNoFenceLines(t *testing.T) {
	in := baseInput()
	in.Name = "```\n```arrow"
	in.Description = "line one\n```\n```arrow\nschema: evil"
	in.URL = "```"
	in.Media = domain.ArrowMedia{Icon: "```", Banner: "\n```\n"}
	in.Generator.Warnings = []string{"```", "```arrow"}
	data, err := forge.Render(in)
	require.NoError(t, err)

	block := yamlBlock(t, data)
	arrow := parse(t, data)

	for _, line := range strings.Split(block, "\n") {
		assert.False(t, strings.HasPrefix(line, "```"), "yaml line %q starts a fence", line)
	}
	assert.Equal(t, "line one ``` ```arrow schema: evil", arrow.Description)
	assert.Equal(t, "``` ```arrow", arrow.Name)
	assert.Equal(t, domain.ArrowMedia{Icon: "```", Banner: "```"}, arrow.Media)
	assert.Equal(t, []string{"```", "```arrow"}, arrow.Generator.Warnings)
}
