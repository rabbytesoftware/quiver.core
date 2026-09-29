package ownership

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
)

func testMarker() Marker {
	return Marker{Owner: "K=", Workdir: "W="}
}

func TestWorkdirOwner(t *testing.T) {
	nsDir := filepath.Join(t.TempDir(), "namespaces")

	testCases := []struct {
		name   string
		target string
		want   domain.Namespace
	}{
		{
			name:   "versioned workdir",
			target: filepath.Join(nsDir, "github.com", "u", "r@v1", "bin", "rg"),
			want:   "github.com/u/r",
		},
		{
			name:   "workdir itself",
			target: filepath.Join(nsDir, "github.com", "u", "r@v1"),
			want:   "github.com/u/r",
		},
		{
			name:   "quiver hosted",
			target: filepath.Join(nsDir, "q.io", "u", "r", "auid@v2", "x"),
			want:   "q.io/u/r/auid",
		},
		{
			name:   "ref with slash",
			target: filepath.Join(nsDir, "github.com", "u", "r@feature", "x", "rg"),
			want:   "github.com/u/r",
		},
		{
			name:   "no ref",
			target: filepath.Join(nsDir, "github.com", "u", "r", "rg"),
			want:   "",
		},
		{
			name:   "outside",
			target: filepath.Join(filepath.Dir(nsDir), "elsewhere@v1", "rg"),
			want:   "",
		},
		{
			name:   "namespaces dir",
			target: nsDir,
			want:   "",
		},
		{
			name:   "relative",
			target: "relative@v1/rg",
			want:   "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, WorkdirOwner(nsDir, tc.target))
		})
	}
}

func TestMarkerValue(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		want    string
		wantOK  bool
	}{
		{name: "crlf", content: "@echo off\r\nREM quiver:github.com/u/r\r\n", want: "github.com/u/r", wantOK: true},
		{name: "lf", content: "a\nREM quiver: github.com/u/r \nb", want: "github.com/u/r", wantOK: true},
		{name: "absent", content: "@echo off\r\n", want: "", wantOK: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := markerValue(tc.content, "REM quiver:")
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantOK, ok)
		})
	}
}

func TestMarker_LinesRoundTrip(t *testing.T) {
	m := Marker{Owner: "K=", Workdir: "W="}

	content := m.Lines(mocks.BareA, "/ns/github.com/acme/tool@v1", "\r\n")

	assert.Equal(t, "K=github.com/acme/tool\r\nW=/ns/github.com/acme/tool@v1\r\n", content)
	assert.Equal(t, Holder{Exists: true, Namespace: mocks.BareA, Target: "/ns/github.com/acme/tool@v1"}, m.holder(content))
}

func TestNamespaceClaim(t *testing.T) {
	claim := NamespaceClaim(mocks.BareA)

	assert.True(t, claim(Holder{Exists: true, Namespace: mocks.BareA, Target: "/anywhere"}))
	assert.False(t, claim(Holder{Exists: true, Namespace: mocks.BareB}))
	assert.False(t, claim(Holder{Exists: true}))
}

func TestWorkdirClaim(t *testing.T) {
	nsDir := t.TempDir()
	v1 := filepath.Join(nsDir, "github.com", "acme", "tool@v1")
	v2 := filepath.Join(nsDir, "github.com", "acme", "tool@v2")
	mocks.WriteFile(t, filepath.Join(v1, "tool"), "v1", 0o600)
	mocks.WriteFile(t, filepath.Join(v2, "tool"), "v2", 0o600)
	claim := WorkdirClaim(mocks.BareA, v1)

	testCases := []struct {
		name string
		h    Holder
		want bool
	}{
		{name: "entry pointing into the workdir", h: Holder{Exists: true, Namespace: mocks.BareA, Target: filepath.Join(v1, "tool")}, want: true},
		{name: "marker naming the workdir", h: Holder{Exists: true, Namespace: mocks.BareA, Target: v1}, want: true},
		{name: "entry pointing into a sibling ref's live workdir", h: Holder{Exists: true, Namespace: mocks.BareA, Target: filepath.Join(v2, "tool")}},
		{name: "marker naming a sibling ref's live workdir", h: Holder{Exists: true, Namespace: mocks.BareA, Target: v2}},
		{name: "entry pointing into a vanished workdir", h: Holder{Exists: true, Namespace: mocks.BareA, Target: filepath.Join(nsDir, "github.com", "acme", "tool@v0", "tool")}, want: true},
		{name: "entry recording no workdir", h: Holder{Exists: true, Namespace: mocks.BareA}, want: true},
		{name: "another namespace pointing into the workdir", h: Holder{Exists: true, Namespace: mocks.BareB, Target: filepath.Join(v1, "tool")}},
		{name: "unmanaged entry", h: Holder{Exists: true}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, claim(tc.h))
		})
	}
}

func TestFileHolder(t *testing.T) {
	dir := t.TempDir()
	marked := filepath.Join(dir, "marked")
	require.NoError(t, os.WriteFile(marked, []byte("K=github.com/u/r\nW=/ns/u/r@v1\n"), 0o600))
	plain := filepath.Join(dir, "plain")
	require.NoError(t, os.WriteFile(plain, []byte("hello"), 0o600))

	testCases := []struct {
		name string
		path string
		want Holder
	}{
		{name: "absent", path: filepath.Join(dir, "missing"), want: Holder{}},
		{name: "directory", path: dir, want: Holder{Exists: true}},
		{name: "marked", path: marked, want: Holder{Exists: true, Namespace: "github.com/u/r", Target: "/ns/u/r@v1"}},
		{name: "plain", path: plain, want: Holder{Exists: true}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FileHolder(tc.path, testMarker())
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFileHolder_Errors(t *testing.T) {
	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable files")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	unreadable := filepath.Join(dir, "unreadable")
	require.NoError(t, os.WriteFile(unreadable, []byte("x"), 0o600))
	require.NoError(t, os.Chmod(unreadable, 0o000))

	testCases := []struct {
		name string
		path string
	}{
		{name: "lstat fails", path: filepath.Join(file, "child")},
		{name: "read fails", path: unreadable},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FileHolder(tc.path, testMarker())
			require.Error(t, err)
		})
	}
}

func TestHolder_Refusal(t *testing.T) {
	testCases := []struct {
		name string
		h    Holder
		want string
	}{
		{name: "absent", h: Holder{}, want: ""},
		{name: "same owner", h: Holder{Exists: true, Namespace: mocks.BareA}, want: ""},
		{name: "unmanaged", h: Holder{Exists: true}, want: models.ReasonUnmanaged},
		{name: "foreign", h: Holder{Exists: true, Namespace: mocks.BareB}, want: "owned by github.com/other/thing"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.h.Refusal(mocks.BareA))
		})
	}
}

func TestRemoveMarked(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, "pre-own.ext")
	foreign := filepath.Join(dir, "pre-foreign.ext")
	plain := filepath.Join(dir, "pre-plain.ext")
	wrongName := filepath.Join(dir, "other.ext")
	kept := filepath.Join(dir, "pre-kept.ext")
	tooShort := filepath.Join(dir, "pre-.ext")
	mocks.WriteFile(t, own, "K=github.com/acme/tool\n", 0o600)
	mocks.WriteFile(t, foreign, "K=github.com/other/thing\n", 0o600)
	mocks.WriteFile(t, plain, "hello", 0o600)
	mocks.WriteFile(t, wrongName, "K=github.com/acme/tool\n", 0o600)
	mocks.WriteFile(t, kept, "K=github.com/acme/tool\n", 0o600)
	mocks.WriteFile(t, tooShort, "K=github.com/acme/tool\n", 0o600)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pre-dir.ext"), 0o750))

	require.NoError(t, RemoveMarked(dir, "pre-", ".ext", testMarker(), NamespaceClaim(mocks.BareA), map[string]bool{kept: true}))

	assert.NoFileExists(t, own)
	for _, survivor := range []string{foreign, plain, wrongName, kept, tooShort} {
		assert.FileExists(t, survivor)
	}
}

func TestRemoveMarked_Errors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	assert.NoError(t, RemoveMarked(filepath.Join(t.TempDir(), "missing"), "", ".x", testMarker(), NamespaceClaim(mocks.BareA), nil))
	assert.Error(t, RemoveMarked(file, "", ".x", testMarker(), NamespaceClaim(mocks.BareA), nil))

	mocks.RequireUnixHost(t)
	if os.Geteuid() == 0 {
		return
	}
	dir := t.TempDir()
	unreadable := filepath.Join(dir, "x.x")
	mocks.WriteFile(t, unreadable, "K=github.com/acme/tool\n", 0o000)

	assert.Error(t, RemoveMarked(dir, "", ".x", testMarker(), NamespaceClaim(mocks.BareA), nil))

	require.NoError(t, os.Chmod(unreadable, 0o600))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	assert.Error(t, RemoveMarked(dir, "", ".x", testMarker(), NamespaceClaim(mocks.BareA), nil))
}
