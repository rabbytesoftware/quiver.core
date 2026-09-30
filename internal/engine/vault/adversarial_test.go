package vault

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// TestAdversarial_WorkDir_NestedSelectorIsInsideAnotherIdentity: a tag
// "release" (pin or pointer channel) and a branch "release/1.0" are both
// legal in one repository and are two identities, yet the second's workdir
// lies inside the first's, so uninstalling crowbar@release deletes
// crowbar@release/1.0's installed files.
func TestAdversarial_WorkDir_NestedSelectorIsInsideAnotherIdentity(t *testing.T) {
	v := newTestVault(t)
	ctx := context.Background()
	outer := domain.Namespace("github.com/u/crowbar@release")
	inner := domain.Namespace("github.com/u/crowbar@release/1.0")

	innerDir, err := v.WorkDir(ctx, inner)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(innerDir, "installed.bin"), []byte("bits"), 0o600))
	_, err = v.WorkDir(ctx, outer)
	require.NoError(t, err)

	require.NoError(t, v.DeleteWorkDir(ctx, outer))

	_, statErr := os.Stat(filepath.Join(innerDir, "installed.bin"))
	assert.NoError(t, statErr, "removing crowbar@release must not delete crowbar@release/1.0's workdir")
}

// TestAdversarial_Identities_DifferingOnlyByCaseShareDisk: git refs are case
// sensitive, so tags "Nightly" and "nightly" are two pointer channels and two
// identities, but their workdirs and manifest cache files differ only by
// case, which is one path on the default macOS and Windows filesystems.
func TestAdversarial_Identities_DifferingOnlyByCaseShareDisk(t *testing.T) {
	s := newTestStore(t)
	upper := domain.Namespace("github.com/u/crowbar@Nightly")
	lower := domain.Namespace("github.com/u/crowbar@nightly")

	upperDir, err := s.namespacePath(upper)
	require.NoError(t, err)
	lowerDir, err := s.namespacePath(lower)
	require.NoError(t, err)
	assert.False(t, strings.EqualFold(upperDir, lowerDir),
		"two identities must never map to workdirs a case-insensitive filesystem treats as one: %s vs %s", upperDir, lowerDir)
	assert.False(t, strings.EqualFold(s.manifestFilePath(upper, "ARROW.md"), s.manifestFilePath(lower, "ARROW.md")),
		"two identities must never share a manifest cache file on a case-insensitive filesystem")
}

// TestAdversarial_Identities_GlobAndItsEscapeStayApart proves v1.* and the
// literal selector v1.%2A never share a workdir or a cache file.
func TestAdversarial_Identities_GlobAndItsEscapeStayApart(t *testing.T) {
	s := newTestStore(t)
	glob := domain.Namespace("github.com/u/crowbar@v1.*")
	literal := domain.Namespace("github.com/u/crowbar@v1.%2A")

	globDir, err := s.namespacePath(glob)
	require.NoError(t, err)
	literalDir, err := s.namespacePath(literal)
	require.NoError(t, err)
	assert.NotEqual(t, globDir, literalDir)
	assert.NotEqual(t, s.manifestFilePath(glob, "ARROW.md"), s.manifestFilePath(literal, "ARROW.md"))
	assert.Equal(t, glob, decodeNSDir(strings.TrimPrefix(filepath.ToSlash(globDir), filepath.ToSlash(s.namespacesPath)+"/")))
	assert.Equal(t, literal, decodeNSDir(strings.TrimPrefix(filepath.ToSlash(literalDir), filepath.ToSlash(s.namespacesPath)+"/")))
}

// TestAdversarial_PutArrow_LongSelectorIsCachable: git allows tag names far
// longer than a filesystem component; an identity on such a tag must still
// be cachable, since every add caches its manifest.
func TestAdversarial_PutArrow_LongSelectorIsCachable(t *testing.T) {
	v := newTestVault(t)
	ns := domain.Namespace("github.com/u/crowbar@release-" + strings.Repeat("x", 240))

	err := v.PutArrow(context.Background(), ns, ManifestFile{Content: []byte("# m"), Filename: "ARROW.md"})
	assert.NoError(t, err, "a 248-character tag is a legal git ref")
}
