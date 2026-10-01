package vault

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	goruntime "runtime"
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
			assert.False(t, strings.ContainsAny(got, `<>:"|?*\/`), "reserved character in %q", got)
			assert.False(t, strings.HasSuffix(got, "."), "trailing dot in %q", got)
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

func TestEncodeNS_PlainRefLayoutUnchanged(t *testing.T) {
	for _, ns := range []domain.Namespace{"github.com/u/r@v1.2.0", "github.com/u/r@stable", "github.com/u/r@nightly-latest", "github.com/u/r"} {
		assert.Equal(t, url.PathEscape(string(ns)), encodeNS(ns), "a plain lower-case ref keeps its cache name: %s", ns)
		assert.Equal(t, legacyEncodeNS(ns), encodeNS(ns))
	}
}

func TestEncodeRefSegment(t *testing.T) {
	testCases := []struct {
		name string
		ref  string
		want string
	}{
		{name: "tag unchanged", ref: "v1.2.0", want: "v1.2.0"},
		{name: "slash is one segment", ref: "release/1.0", want: "release%2F1.0"},
		{name: "upper case is escaped", ref: "Nightly", want: "%4Eightly"},
		{name: "non-ascii is escaped", ref: "caf\xc3\xa9", want: "caf%C3%A9"},
		{name: "tilde is escaped", ref: "a~b", want: "a%7Eb"},
		{name: "percent is escaped", ref: "50%", want: "50%25"},
		{name: "trailing dot", ref: "v1.", want: "v1%2E"},
		{name: "dot dot", ref: "..", want: ".%2E"},
		{name: "wildcard", ref: "v1.*", want: "v1.%2A"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := encodeRefSegment(tc.ref)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.ref, decodeNSDir("h/u/r@"+got).Ref())
		})
	}
}

func TestCapName(t *testing.T) {
	long := domain.Namespace("github.com/u/r@release-" + strings.Repeat("x", 240))
	other := domain.Namespace("github.com/u/r@release-" + strings.Repeat("x", 239) + "y")
	escaped := domain.Namespace("github.com/u/r@" + strings.Repeat("X", 120))

	testCases := []struct {
		name string
		ns   domain.Namespace
	}{
		{name: "long plain selector", ns: long},
		{name: "long selector differing only at the end", ns: other},
		{name: "long escaped selector never splits an escape", ns: escaped},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := encodeNS(tc.ns)
			assert.LessOrEqual(t, len(got), maxNameLen)
			assert.True(t, isHashed(got))
			prefix, _, _ := strings.Cut(got, hashedMarker)
			_, err := url.PathUnescape(prefix)
			assert.NoError(t, err, "the cut keeps every escape whole: %q", prefix)
		})
	}
	assert.NotEqual(t, encodeNS(long), encodeNS(other))
	assert.False(t, isHashed(encodeNS("github.com/u/r@v1.2.0")))
}

func TestVault_LongSelector_CachesListsAndSweeps(t *testing.T) {
	vaultDir, nsDir := t.TempDir(), t.TempDir()
	base := time.Now()
	v, err := NewWithClock(vaultDir, nsDir, time.Hour, func() time.Time { return base })
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	ns := domain.Namespace("github.com/u/r@release-" + strings.Repeat("x", 240))

	require.NoError(t, v.PutArrow(context.Background(), ns, testManifest))
	got, err := v.GetArrow(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, testManifest.Content, got.Content)
	versions, err := v.ListVersions(context.Background(), ns.BareNamespace())
	require.NoError(t, err)
	assert.Equal(t, []string{ns.Ref()}, versions)

	dir, err := v.WorkDir(context.Background(), ns)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(filepath.Base(dir)), maxNameLen)

	later, err := NewWithClock(vaultDir, nsDir, time.Hour, func() time.Time { return base.Add(2 * time.Hour) })
	require.NoError(t, err)
	t.Cleanup(func() { _ = later.Close() })
	later.(*store).sweepArrows()
	_, err = later.GetArrow(context.Background(), ns)
	assert.ErrorIs(t, err, ErrNotCached)
}

// An install made under an earlier layout keeps its workdir: a slash or an
// upper-case selector was laid out differently, and moving the files would
// lose what the arrow installed.
func TestVault_LegacyWorkDir_StillUsed(t *testing.T) {
	testCases := []struct {
		name   string
		ns     domain.Namespace
		legacy string
	}{
		{name: "branch with slash", ns: "github.com/u/r@feat/x", legacy: "github.com/u/r@feat/x"},
		{name: "upper case tag", ns: "github.com/u/r@Nightly", legacy: "github.com/u/r@Nightly"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			nsDir := t.TempDir()
			v, err := New(t.TempDir(), nsDir, time.Hour)
			require.NoError(t, err)
			t.Cleanup(func() { _ = v.Close() })
			legacy := filepath.Join(nsDir, filepath.FromSlash(tc.legacy))
			require.NoError(t, os.MkdirAll(legacy, 0o700))

			dir, err := v.WorkDir(context.Background(), tc.ns)
			require.NoError(t, err)
			assert.Equal(t, legacy, dir)
		})
	}
}

func TestVault_NewWorkDir_IsOneFlatSegment(t *testing.T) {
	nsDir := t.TempDir()
	v, err := New(t.TempDir(), nsDir, time.Hour)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })

	dir, err := v.WorkDir(context.Background(), "github.com/u/r@feat/x")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(nsDir, "github.com", "u", "r@feat%2Fx"), dir)
}

// Another identity's directory that differs only by case is not a legacy
// workdir of this one, even where the filesystem folds case.
func TestVault_LegacyWorkDir_CaseFoldedSiblingIsNotAdopted(t *testing.T) {
	nsDir := t.TempDir()
	v, err := New(t.TempDir(), nsDir, time.Hour)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	lower, err := v.WorkDir(context.Background(), "github.com/u/r@nightly")
	require.NoError(t, err)

	upper, err := v.WorkDir(context.Background(), "github.com/u/r@Nightly")
	require.NoError(t, err)
	assert.False(t, strings.EqualFold(lower, upper))
}

func TestVault_LegacyCacheEntry_ReadAndDeleted(t *testing.T) {
	vaultDir := t.TempDir()
	v, err := New(vaultDir, t.TempDir(), time.Hour)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	ns := domain.Namespace("github.com/u/r@Nightly")
	legacy := legacyEncodeNS(ns)
	require.NotEqual(t, encodeNS(ns), legacy)
	meta := []byte(`{"cached_at":"` + time.Now().UTC().Format(time.RFC3339) + `","filename":"ARROW.md"}`)
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, legacy+metaSuffix), meta, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, legacy+".md"), []byte("# legacy"), 0o600))

	got, err := v.GetArrow(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, []byte("# legacy"), got.Content)

	require.NoError(t, v.DeleteArrow(context.Background(), ns))
	assert.NoFileExists(t, filepath.Join(vaultDir, legacy+metaSuffix))
	assert.NoFileExists(t, filepath.Join(vaultDir, legacy+".md"))
}

func TestVault_BareNamespaceClimbingOut_LegacyLayoutRejected(t *testing.T) {
	v := newTestVault(t)
	_, err := v.WorkDir(context.Background(), "../../x@A/b")
	require.ErrorIs(t, err, ErrInvalidNamespace)
}

func TestVault_LongSelectorCollection_ListsAndSweeps(t *testing.T) {
	nsDir := t.TempDir()
	base := time.Now()
	v, err := NewWithClock(t.TempDir(), nsDir, time.Hour, func() time.Time { return base })
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	ns := domain.Namespace("github.com/u/c@release-" + strings.Repeat("x", 240))

	path, err := v.PutCollection(context.Background(), ns, &domain.Collection{Namespace: ns})
	require.NoError(t, err)

	listed, err := v.ListCachedCollections(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []domain.Namespace{ns}, listed)

	later, err := NewWithClock(t.TempDir(), nsDir, time.Hour, func() time.Time { return base.Add(2 * time.Hour) })
	require.NoError(t, err)
	t.Cleanup(func() { _ = later.Close() })
	later.(*store).sweepQuivers()
	assert.NoFileExists(t, path)
}

func TestCollectionNamespace_UnreadableCappedFileFallsBackToTheName(t *testing.T) {
	dir := t.TempDir()
	rel := "github.com/u/c@x" + hashedMarker + "abc"
	missing := filepath.Join(dir, "missing.json")
	corrupt := filepath.Join(dir, "corrupt.json")
	require.NoError(t, os.WriteFile(corrupt, []byte("{"), 0o600))

	assert.Equal(t, decodeNSDir(rel), collectionNamespace(rel, missing))
	assert.Equal(t, decodeNSDir(rel), collectionNamespace(rel, corrupt))
}

func caseInsensitive(t *testing.T, dir string) bool {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Probe"), nil, 0o600))
	_, err := os.Stat(filepath.Join(dir, "probe"))
	require.NoError(t, os.Remove(filepath.Join(dir, "Probe")))
	return err == nil
}

// On a case-insensitive filesystem the plain v1.0 identity's path is the
// directory an earlier layout gave V1.0; it must never be handed to v1.0,
// where one uninstall would delete the other's files.
func TestVault_CaseFoldedLegacyDirectory_IsNeverShared(t *testing.T) {
	nsDir := t.TempDir()
	if !caseInsensitive(t, nsDir) {
		t.Skip("the filesystem tells the two spellings apart")
	}
	v, err := New(t.TempDir(), nsDir, time.Hour)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	legacy := filepath.Join(nsDir, "github.com", "u", "r@V1.0")
	require.NoError(t, os.MkdirAll(legacy, 0o700))

	upper, err := v.WorkDir(context.Background(), "github.com/u/r@V1.0")
	require.NoError(t, err)
	assert.Equal(t, legacy, upper, "V1.0 keeps its own legacy workdir")

	_, err = v.WorkDir(context.Background(), "github.com/u/r@v1.0")
	require.ErrorIs(t, err, ErrWorkDirCollision)

	require.NoError(t, v.DeleteWorkDir(context.Background(), "github.com/u/r@v1.0"),
		"removing the identity that owns no directory must succeed")
	assert.DirExists(t, legacy, "and must leave the other identity's files alone")
}

// A constraint identity catalogued before paths were escaped has its
// workdir under the raw selector.
func TestVault_RawLegacyWorkDir_StillUsed(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("windows could never hold the raw selector")
	}
	nsDir := t.TempDir()
	v, err := New(t.TempDir(), nsDir, time.Hour)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	raw := filepath.Join(nsDir, "github.com", "u", "r@v1.*")
	require.NoError(t, os.MkdirAll(raw, 0o700))

	dir, err := v.WorkDir(context.Background(), "github.com/u/r@v1.*")

	require.NoError(t, err)
	assert.Equal(t, raw, dir)
}

// Every identity segment stays short enough that a workdir under a typical
// Windows home keeps room under MAX_PATH for the arrow's own files.
func TestVault_IdentitySegment_FitsWindowsPaths(t *testing.T) {
	nsDir := t.TempDir()
	v, err := New(t.TempDir(), nsDir, time.Hour)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	s := v.(*store)

	long := domain.Namespace("github.com/u/r@release-" + strings.Repeat("x", 120))
	dir, err := v.WorkDir(context.Background(), long)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(filepath.Base(dir)), 96)
	assert.LessOrEqual(t, len(filepath.Base(s.metaFilePath(long))), 96+len(metaSuffix))

	short := domain.Namespace("github.com/u/r@release-2026-09-27.1")
	dir, err = v.WorkDir(context.Background(), short)
	require.NoError(t, err)
	assert.Equal(t, "r@release-2026-09-27.1", filepath.Base(dir), "a plain ref keeps its spelling")
}
