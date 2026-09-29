package lnk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/discover"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

type fixture struct {
	*mocks.Sandbox
	exposer *exposer
	dir     string
}

func newFixture(
	t *testing.T,
) *fixture {
	t.Helper()
	sb := mocks.NewSandbox(t, "windows")
	return &fixture{
		Sandbox: sb,
		exposer: New(discover.Scan{Rule: discover.ExeSuffix()}).(*exposer),
		dir:     startMenuDir(filepath.Join(sb.UserHome, "AppData", "Roaming")),
	}
}

func (f *fixture) place(
	req models.Request,
	entry domain.ExposeEntry,
	c models.Candidate,
) (models.Placement, error) {
	return f.exposer.Place(context.Background(), req, entry, c)
}

func (f *fixture) remove(
	t *testing.T,
	claim models.Claim,
	keep map[string]bool,
) error {
	t.Helper()
	return f.exposer.Remove(context.Background(), f.Layout(t), claim, keep)
}

func writeLink(
	t *testing.T,
	path string,
	l Link,
) {
	t.Helper()
	data, err := Encode(l)
	require.NoError(t, err)
	mocks.WriteFile(t, path, string(data), 0o600)
}

func readBack(
	t *testing.T,
	path string,
) Link {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test reads the shortcut it placed
	require.NoError(t, err)
	l, err := Decode(data)
	require.NoError(t, err)
	return l
}

func TestExposer_Find(t *testing.T) {
	f := newFixture(t)
	req, wd := f.Request(t, mocks.NsA)
	mocks.WriteFile(t, filepath.Join(wd, "setup.exe"), "x", 0o644)

	got, reason, err := f.exposer.Find(req, domain.ExposeEntry{Name: "Tool", Path: domain.ExposeAuto})

	require.NoError(t, err)
	assert.Empty(t, reason)
	assert.Equal(t, []models.Candidate{{Name: "Tool", Target: filepath.Join(wd, "setup.exe"), Depth: 1}}, got)
}

func TestExposer_Place(t *testing.T) {
	f := newFixture(t)
	req, wd := f.Request(t, mocks.NsA)
	target := filepath.Join(wd, "tool.exe")
	mocks.WriteFile(t, target, "x", 0o644)
	loc := filepath.Join(f.dir, "Tool.lnk")

	testCases := []struct {
		name     string
		existing func(t *testing.T)
		target   string
		want     models.Placement
	}{
		{name: "new shortcut", target: target, want: models.Placement{Location: loc}},
		{
			name: "own shortcut",
			existing: func(t *testing.T) {
				writeLink(t, loc, Link{Target: `C:\old.exe`, Description: description(mocks.BareA, `C:\old`)})
			},
			target: target,
			want:   models.Placement{Location: loc},
		},
		{
			name: "foreign shortcut",
			existing: func(t *testing.T) {
				writeLink(t, loc, Link{Target: `C:\x.exe`, Description: description(mocks.BareB, `C:\x`)})
			},
			target: target,
			want:   models.Placement{Refused: "owned by github.com/other/thing"},
		},
		{
			name:     "user shortcut",
			existing: func(t *testing.T) { writeLink(t, loc, Link{Target: `C:\x.exe`, Description: "Some App"}) },
			target:   target,
			want:     models.Placement{Refused: models.ReasonUnmanaged},
		},
		{
			name:     "file that is not a shortcut",
			existing: func(t *testing.T) { mocks.WriteFile(t, loc, "not a link", 0o600) },
			target:   target,
			want:     models.Placement{Refused: models.ReasonUnmanaged},
		},
		{
			name:     "directory in the way",
			existing: func(t *testing.T) { require.NoError(t, os.MkdirAll(loc, 0o750)) },
			target:   target,
			want:     models.Placement{Refused: models.ReasonUnmanaged},
		},
		{name: "missing target", target: filepath.Join(wd, "missing.exe"), want: models.Placement{Refused: models.ReasonNotFound}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.RemoveAll(loc))
			if tc.existing != nil {
				tc.existing(t)
			}

			got, err := f.place(req, domain.ExposeEntry{}, models.Candidate{Name: "Tool", Target: tc.target})

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			if tc.want.Location == "" {
				return
			}
			assert.Equal(t, Link{Target: target, WorkingDir: wd, Description: "quiver:github.com/acme/tool|" + wd}, readBack(t, loc))
		})
	}
}

func TestExposer_Place_KeepsOnlyIcoIcons(t *testing.T) {
	f := newFixture(t)
	req, wd := f.Request(t, mocks.NsA)
	target := filepath.Join(wd, "it's.exe")
	mocks.WriteFile(t, target, "x", 0o644)

	testCases := []struct {
		name string
		icon string
		want string
	}{
		{name: "ico icon", icon: "${INSTALL_PATH}/tool.ICO", want: filepath.Join(wd, "tool.ICO")},
		{name: "png icon is dropped", icon: "${INSTALL_PATH}/tool.png", want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := f.place(req, domain.ExposeEntry{Icon: tc.icon}, models.Candidate{Name: "Tool", Target: target})

			require.NoError(t, err)
			assert.Equal(t, tc.want, readBack(t, got.Location).Icon)
		})
	}
}

func TestExposer_Place_Errors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	testCases := []struct {
		name  string
		setup func(t *testing.T, f *fixture, req *models.Request)
	}{
		{
			name:  "start menu folder not creatable",
			setup: func(_ *testing.T, _ *fixture, req *models.Request) { req.Layout.AppData = file },
		},
		{
			name:  "description too long",
			setup: func(_ *testing.T, _ *fixture, req *models.Request) { req.Workdir = strings.Repeat("x", maxStringChars) },
		},
		{
			name: "staged file stuck",
			setup: func(t *testing.T, f *fixture, _ *models.Request) {
				mocks.WriteFile(t, filepath.Join(f.dir, "Tool.lnk"+fsguard.StagedSuffix, "x"), "", 0o600)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			req, wd := f.Request(t, mocks.NsA)
			target := filepath.Join(wd, "tool.exe")
			mocks.WriteFile(t, target, "x", 0o644)
			tc.setup(t, f, &req)

			_, err := f.place(req, domain.ExposeEntry{}, models.Candidate{Name: "Tool", Target: target})

			require.Error(t, err)
		})
	}
}

func TestExposer_Remove(t *testing.T) {
	f := newFixture(t)
	own := filepath.Join(f.dir, "own.lnk")
	foreign := filepath.Join(f.dir, "foreign.LNK")
	user := filepath.Join(f.dir, "user.lnk")
	notes := filepath.Join(f.dir, "notes.txt")
	kept := filepath.Join(f.dir, "own-kept.lnk")
	writeLink(t, own, Link{Target: `C:\a.exe`, Description: description(mocks.BareA, `C:\wd`)})
	writeLink(t, foreign, Link{Target: `C:\a.exe`, Description: description(mocks.BareB, `C:\wd`)})
	writeLink(t, user, Link{Target: `C:\a.exe`, Description: "mine"})
	writeLink(t, kept, Link{Target: `C:\a.exe`, Description: description(mocks.BareA, `C:\wd`)})
	mocks.WriteFile(t, notes, description(mocks.BareA, `C:\wd`), 0o600)
	require.NoError(t, os.MkdirAll(filepath.Join(f.dir, "sub.lnk"), 0o750))

	require.NoError(t, f.remove(t, models.NamespaceClaim(mocks.BareA), map[string]bool{kept: true}))

	assert.NoFileExists(t, own)
	for _, survivor := range []string{kept, foreign, user, notes} {
		assert.FileExists(t, survivor)
	}
}

func TestExposer_Remove_WorkdirClaimSparesSiblingRef(t *testing.T) {
	f := newFixture(t)
	v1 := f.Workdir(t, mocks.NsA)
	v2 := f.Workdir(t, mocks.NsA2)
	mine := filepath.Join(f.dir, "mine.lnk")
	sibling := filepath.Join(f.dir, "sibling.lnk")
	writeLink(t, mine, Link{Target: `C:\a.exe`, Description: description(mocks.BareA, v1)})
	writeLink(t, sibling, Link{Target: `C:\a.exe`, Description: description(mocks.BareA, v2)})

	require.NoError(t, f.remove(t, models.WorkdirClaim(mocks.BareA, v1), nil))

	assert.NoFileExists(t, mine)
	assert.FileExists(t, sibling)
}

func TestExposer_Remove_Errors(t *testing.T) {
	f := newFixture(t)
	claim := models.NamespaceClaim(mocks.BareA)
	missing := models.Layout{AppData: filepath.Join(t.TempDir(), "missing")}
	blocked := t.TempDir()
	mocks.WriteFile(t, startMenuDir(blocked), "", 0o600)

	assert.NoError(t, f.exposer.Remove(context.Background(), missing, claim, nil))
	assert.Error(t, f.exposer.Remove(context.Background(), models.Layout{AppData: blocked}, claim, nil))

	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		return
	}
	appData := t.TempDir()
	dir := startMenuDir(appData)
	unreadable := filepath.Join(dir, "x.lnk")
	writeLink(t, unreadable, Link{Target: `C:\a.exe`, Description: description(mocks.BareA, `C:\wd`)})
	require.NoError(t, os.Chmod(unreadable, 0o000))

	assert.Error(t, f.exposer.Remove(context.Background(), models.Layout{AppData: appData}, claim, nil))

	require.NoError(t, os.Chmod(unreadable, 0o600))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	assert.Error(t, f.exposer.Remove(context.Background(), models.Layout{AppData: appData}, claim, nil))
}

func TestHolder(t *testing.T) {
	dir := t.TempDir()
	owned := filepath.Join(dir, "owned.lnk")
	writeLink(t, owned, Link{Target: `C:\a.exe`, Description: "quiver:github.com/acme/tool|C:\\wd\r\n"})

	testCases := []struct {
		name string
		loc  string
		want ownership.Holder
	}{
		{name: "absent", loc: filepath.Join(dir, "missing.lnk"), want: ownership.Holder{}},
		{name: "owned", loc: owned, want: ownership.Holder{Exists: true, Namespace: mocks.BareA, Target: `C:\wd`}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := holder(tc.loc)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestHolder_InspectError(t *testing.T) {
	mocks.RequireUnixHost(t)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := holder(filepath.Join(file, "x.lnk"))

	require.Error(t, err)
}
