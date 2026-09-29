package vault

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// assertWindowsSafe fails if any component of path below base is a name
// Windows would reject or silently rewrite.
func assertWindowsSafe(t *testing.T, base, path string) {
	t.Helper()
	rel, err := filepath.Rel(base, path)
	require.NoError(t, err)
	require.False(t, strings.HasPrefix(rel, ".."), "path %q escapes %q", path, base)
	for _, component := range strings.Split(filepath.ToSlash(rel), "/") {
		assert.NotContains(t, component, "*")
		assert.False(t, strings.ContainsAny(component, `<>:"|?*\`), "reserved character in %q", component)
		assert.False(t, strings.HasSuffix(component, ".") || strings.HasSuffix(component, " "),
			"trailing dot or space in %q", component)
	}
}

func TestEscapeRefDir(t *testing.T) {
	testCases := []struct {
		name string
		ref  string
		want string
	}{
		{name: "tag unchanged", ref: "v1.2.0", want: "v1.2.0"},
		{name: "rolling tag unchanged", ref: "nightly", want: "nightly"},
		{name: "channel unchanged", ref: "stable", want: "stable"},
		{name: "dash underscore dot unchanged", ref: "release-1_2.x", want: "release-1_2.x"},
		{name: "branch with slash keeps its nesting", ref: "feat/x", want: "feat/x"},
		{name: "device-like first component unchanged", ref: "aux", want: "aux"},
		{name: "wildcard selector", ref: "v1.*", want: "v1.%2A"},
		{name: "colon", ref: "a:b", want: "a%3Ab"},
		{name: "every windows reserved character", ref: `<>:"|?*\`, want: "%3C%3E%3A%22%7C%3F%2A%5C"},
		{name: "percent is escaped so decoding is unambiguous", ref: "50%", want: "50%25"},
		{name: "control character", ref: "a\tb", want: "a%09b"},
		{name: "dot dot component cannot climb", ref: "..", want: ".%2E"},
		{name: "dot component", ref: ".", want: "%2E"},
		{name: "dot dot inside a path", ref: "x/../y", want: "x/.%2E/y"},
		{name: "trailing dot", ref: "v1.", want: "v1%2E"},
		{name: "trailing space", ref: "v1 ", want: "v1%20"},
		{name: "device name after a slash", ref: "feat/con", want: "feat/%63on"},
		{name: "device name with extension after a slash", ref: "feat/LPT1.txt", want: "feat/%4CPT1.txt"},
		{name: "near device name unchanged", ref: "feat/console", want: "feat/console"},
		{name: "empty ref", ref: "", want: ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := escapeRefDir(tc.ref)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.ref, decodeNSDir("h/u/r@"+got).Ref())
		})
	}
}

func TestEncodeNS(t *testing.T) {
	testCases := []struct {
		name string
		ns   domain.Namespace
	}{
		{name: "tag", ns: "github.com/u/r@v1.2.0"},
		{name: "channel", ns: "github.com/u/r@stable"},
		{name: "branch with slash", ns: "github.com/u/r@feat/x"},
		{name: "bare", ns: "github.com/u/r"},
		{name: "empty ref", ns: "github.com/u/r@"},
		{name: "wildcard selector", ns: "github.com/u/r@v1.*"},
		{name: "colon", ns: "github.com/u/r@a:b"},
		{name: "dot dot", ns: "github.com/u/r@.."},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := encodeNS(tc.ns)
			if !strings.Contains(tc.ns.Ref(), ":") {
				assert.Equal(t, url.PathEscape(string(tc.ns)), got, "layout must stay byte-for-byte")
			}
			assert.False(t, strings.ContainsAny(got, `<>:"|?*\/`), "reserved character in %q", got)
			decoded, err := url.PathUnescape(got)
			require.NoError(t, err)
			assert.Equal(t, string(tc.ns), decoded)
		})
	}
}

func TestDecodeNSDir_MalformedEscapeKeptVerbatim(t *testing.T) {
	assert.Equal(t, domain.Namespace("h/u/r@50%zz"), decodeNSDir("h/u/r@50%zz"))
	assert.Equal(t, domain.Namespace("h/u/r"), decodeNSDir("h/u/r"))
}

func TestVault_SelectorIdentity_PathsAreWindowsSafe(t *testing.T) {
	testCases := []struct {
		name string
		ns   domain.Namespace
	}{
		{name: "wildcard", ns: "github.com/u/r@v1.*"},
		{name: "colon", ns: "github.com/u/r@a:b"},
		{name: "branch with dot dot", ns: "github.com/u/r@x/../../../../y"},
		{name: "leading slash", ns: "github.com/u/r@/abs"},
		{name: "dot dot", ns: "github.com/u/r@.."},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vaultDir, nsDir := t.TempDir(), t.TempDir()
			v, err := New(vaultDir, nsDir, time.Hour)
			require.NoError(t, err)
			t.Cleanup(func() { _ = v.Close() })
			s := v.(*store)

			dir, err := v.WorkDir(context.Background(), tc.ns)
			require.NoError(t, err)
			assert.DirExists(t, dir)
			assertWindowsSafe(t, nsDir, dir)

			require.NoError(t, v.PutArrow(context.Background(), tc.ns, testManifest))
			assertWindowsSafe(t, vaultDir, s.metaFilePath(tc.ns))
			assertWindowsSafe(t, vaultDir, s.manifestFilePath(tc.ns, testManifest.Filename))

			got, err := v.GetArrow(context.Background(), tc.ns)
			require.NoError(t, err)
			assert.Equal(t, testManifest.Content, got.Content)

			versions, err := v.ListVersions(context.Background(), tc.ns.BareNamespace())
			require.NoError(t, err)
			assert.Equal(t, []string{tc.ns.Ref()}, versions)

			require.NoError(t, v.DeleteWorkDir(context.Background(), tc.ns))
			assert.NoDirExists(t, dir)
			assert.DirExists(t, nsDir)
		})
	}
}

func TestVault_OrdinaryIdentity_WorkDirLayoutUnchanged(t *testing.T) {
	testCases := []struct {
		name string
		ns   domain.Namespace
	}{
		{name: "tag", ns: "github.com/u/r@v1.2.0"},
		{name: "rolling tag", ns: "github.com/u/r@nightly"},
		{name: "branch with slash", ns: "github.com/u/r@feat/x"},
		{name: "bare", ns: "github.com/u/r"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			nsDir := t.TempDir()
			v, err := New(t.TempDir(), nsDir, time.Hour)
			require.NoError(t, err)
			t.Cleanup(func() { _ = v.Close() })

			dir, err := v.WorkDir(context.Background(), tc.ns)
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(nsDir, filepath.FromSlash(string(tc.ns))), dir)
		})
	}
}

func TestVault_BareNamespaceClimbingOut_Rejected(t *testing.T) {
	v := newTestVault(t)
	ns := domain.Namespace("../../x@v1")

	_, err := v.WorkDir(context.Background(), ns)
	require.ErrorIs(t, err, ErrInvalidNamespace)
	require.ErrorIs(t, v.DeleteWorkDir(context.Background(), ns), ErrInvalidNamespace)
	require.ErrorIs(t, v.PutArrow(context.Background(), ns, testManifest), ErrInvalidNamespace)
}

func TestVault_SelectorCollection_RoundTrips(t *testing.T) {
	nsDir := t.TempDir()
	base := time.Now()
	v, err := NewWithClock(t.TempDir(), nsDir, time.Hour, func() time.Time { return base })
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	ns := domain.Namespace("github.com/u/c@v1.*")

	path, err := v.PutCollection(context.Background(), ns, &domain.Collection{Namespace: ns})
	require.NoError(t, err)
	assertWindowsSafe(t, nsDir, path)

	listed, err := v.ListCachedCollections(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []domain.Namespace{ns}, listed)

	later, err := NewWithClock(t.TempDir(), nsDir, time.Hour, func() time.Time { return base.Add(2 * time.Hour) })
	require.NoError(t, err)
	t.Cleanup(func() { _ = later.Close() })
	later.(*store).sweepQuivers()
	assert.NoFileExists(t, path)
}
