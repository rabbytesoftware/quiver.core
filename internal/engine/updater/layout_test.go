package updater

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestNewLayout_AbsolutePath_ReturnsSystemArrowLayout(t *testing.T) {
	namespaces := t.TempDir()

	layout, err := newLayout(namespaces, "linux")

	require.NoError(t, err)
	wantRoot := filepath.Join(namespaces, filepath.FromSlash(SystemNamespace))
	assert.Equal(t, wantRoot, layout.Root())
	assert.Equal(t, filepath.Join(wantRoot, "versions"), layout.VersionsDir())
	assert.Equal(t, filepath.Join(wantRoot, "update"), layout.UpdateDir())
	assert.Equal(t, filepath.Join(wantRoot, "update", "current.json"), layout.CurrentPath())
	assert.Equal(t, filepath.Join(wantRoot, "update", "staged.json"), layout.StagedPath())
	assert.Equal(t, filepath.Join(wantRoot, "update", "attempt.json"), layout.AttemptPath())
}

func TestNewLayout_Windows_UsesExecutableSuffix(t *testing.T) {
	layout, err := newLayout(t.TempDir(), "windows")
	require.NoError(t, err)

	artifact, err := NewArtifact(layout, "v1.2.3", testDigest)

	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(artifact.Executable, "/quiver.exe"))
}

func TestNewLayout_RuntimeOS_ReturnsLayout(t *testing.T) {
	layout, err := NewLayout(t.TempDir())

	require.NoError(t, err)
	assert.NotEmpty(t, layout.Root())
}

func TestNewLayout_InvalidInputs_ReturnErrors(t *testing.T) {
	testCases := []struct {
		name       string
		namespaces string
		goos       string
	}{
		{name: "empty path", goos: "linux"},
		{name: "relative path", namespaces: "namespaces", goos: "linux"},
		{name: "empty operating system", namespaces: t.TempDir()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			layout, err := newLayout(tc.namespaces, tc.goos)

			require.ErrorIs(t, err, ErrInvalidLayout)
			assert.Empty(t, layout.Root())
		})
	}
}

func TestLayout_ZeroValue_IsRejectedByOperationalMethods(t *testing.T) {
	layout := Layout{}

	artifact, err := NewArtifact(layout, "v1", testDigest)
	require.ErrorIs(t, err, ErrInvalidLayout)
	assert.Empty(t, artifact)
	assert.Empty(t, layout.CurrentPath())
	assert.Empty(t, layout.StagedPath())
	assert.Empty(t, layout.AttemptPath())
}

func TestNewArtifact_ValidIdentity_ReturnsCanonicalRelativePath(t *testing.T) {
	layout, err := newLayout(t.TempDir(), "linux")
	require.NoError(t, err)

	artifact, err := NewArtifact(layout, "v1.2.3-rc.1+build_4", testDigest)

	require.NoError(t, err)
	assert.Equal(t, "v1.2.3-rc.1+build_4", artifact.Version)
	assert.Equal(t, testDigest, artifact.Digest)
	assert.Equal(t, "versions/v1.2.3-rc.1+build_4-"+testDigest+"/quiver", artifact.Executable)
}

func TestNewArtifact_InvalidIdentity_ReturnsError(t *testing.T) {
	layout, err := newLayout(t.TempDir(), "linux")
	require.NoError(t, err)
	testCases := []struct {
		name    string
		version string
		digest  string
	}{
		{name: "empty version", digest: testDigest},
		{name: "version traversal", version: "../v1", digest: testDigest},
		{name: "version too long", version: strings.Repeat("v", MaxVersionLength+1), digest: testDigest},
		{name: "short digest", version: "v1", digest: "abc"},
		{name: "uppercase digest", version: "v1", digest: strings.ToUpper(testDigest)},
		{name: "non hexadecimal digest", version: "v1", digest: strings.Repeat("z", 64)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			artifact, artifactErr := NewArtifact(layout, tc.version, tc.digest)

			require.ErrorIs(t, artifactErr, ErrInvalidState)
			assert.Empty(t, artifact)
		})
	}
}

func TestArtifact_Validate_UnsafePathReturnsError(t *testing.T) {
	layout, err := newLayout(t.TempDir(), "linux")
	require.NoError(t, err)
	artifact, err := NewArtifact(layout, "v1", testDigest)
	require.NoError(t, err)
	artifact.Executable = "../../quiver"

	err = artifact.Validate(layout)

	require.True(t, errors.Is(err, ErrUnsafePath))
}
