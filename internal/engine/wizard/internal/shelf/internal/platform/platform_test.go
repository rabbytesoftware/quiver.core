package platform

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

func forSandbox(
	sb *mocks.Sandbox,
) Platform {
	return ForOS(sb.GOOS, sb.Host(), Seams{Tagger: sb.Tagger, UserPath: sb.UserPath})
}

func TestForOS_ComposesEveryStrategy(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows", "freebsd"} {
		t.Run(goos, func(t *testing.T) {
			p := forSandbox(mocks.NewSandbox(t, goos))

			assert.NotNil(t, p.CLI)
			assert.NotNil(t, p.Desktop)
			assert.NotNil(t, p.Path)
			assert.NotNil(t, p.SafeName)
		})
	}
}

func TestForOS_CLIStrategy(t *testing.T) {
	testCases := []struct {
		name     string
		goos     string
		file     string
		location func(sb *mocks.Sandbox, wd string) string
	}{
		{name: "linux links into the bin dir", goos: "linux", file: "tool", location: func(sb *mocks.Sandbox, _ string) string { return filepath.Join(sb.Bin, "tool") }},
		{name: "darwin links into the bin dir", goos: "darwin", file: "tool", location: func(sb *mocks.Sandbox, _ string) string { return filepath.Join(sb.Bin, "tool") }},
		{name: "windows puts the folder on the user path", goos: "windows", file: "tool.exe", location: func(_ *mocks.Sandbox, wd string) string { return wd }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.goos != "windows" {
				mocks.RequireUnixHost(t)
			}
			sb := mocks.NewSandbox(t, tc.goos)
			req, wd := sb.Request(t, mocks.NsA)
			target := filepath.Join(wd, tc.file)
			mocks.WriteFile(t, target, "x", 0o755)

			got, err := forSandbox(sb).CLI.Place(context.Background(), req, domain.ExposeEntry{}, models.Candidate{Name: "tool", Target: target})

			require.NoError(t, err)
			assert.Equal(t, models.Placement{Location: tc.location(sb, wd)}, got)
		})
	}
}

func TestDarwinSigner_OnlyOnAppleSilicon(t *testing.T) {
	testCases := []struct {
		arch      string
		wantCalls int
	}{
		{arch: "arm64", wantCalls: 1},
		{arch: "amd64", wantCalls: 0},
	}

	for _, tc := range testCases {
		t.Run(tc.arch, func(t *testing.T) {
			sb := mocks.NewSandbox(t, "darwin")
			sb.GOARCH = tc.arch
			wd := sb.Workdir(t, mocks.NsA)
			target := filepath.Join(wd, "tool")
			mocks.WriteFile(t, target, string([]byte{0xcf, 0xfa, 0xed, 0xfe}), 0o755)

			darwinSigner(sb.Host()).Sign(context.Background(), wd, target)

			assert.Len(t, sb.Cmd.Calls, tc.wantCalls)
		})
	}
}

func TestWindowsSafeName(t *testing.T) {
	testCases := []struct {
		name string
		want bool
	}{
		{name: "tool", want: true},
		{name: "My App", want: true},
		{name: "console", want: true},
		{name: "CON", want: false},
		{name: "nul", want: false},
		{name: "com1", want: false},
		{name: "LPT9.txt", want: false},
		{name: "tool.", want: false},
		{name: "tool ", want: false},
		{name: "a/b", want: false},
		{name: "a:b", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, windowsSafeName(tc.name))
		})
	}
}
