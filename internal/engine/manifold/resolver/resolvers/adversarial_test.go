package resolvers

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitCLI(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...) // #nosec G204 -- fixed test arguments
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// lsRemotePeeled is what real git reports each tag names: the peeled
// "<tag>^{}" commit for an annotated tag, else the tag's own hash.
func lsRemotePeeled(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := gitCLI(t, dir, "ls-remote", "--tags", dir)
	tags := map[string]string{}
	peeled := map[string]string{}
	sc := bufio.NewScanner(bytes.NewBufferString(out))
	for sc.Scan() {
		hash, name, ok := strings.Cut(sc.Text(), "\t")
		if !ok {
			continue
		}
		name = strings.TrimPrefix(name, "refs/tags/")
		if tag, isPeeled := strings.CutSuffix(name, "^{}"); isPeeled {
			peeled[tag] = hash
			continue
		}
		tags[name] = hash
	}
	for tag, hash := range peeled {
		tags[tag] = hash
	}
	return tags
}

// TestAdversarial_Refs_AnnotatedRollingTagRecordsTheCommit builds a repo with
// the real git CLI: an annotated rolling tag force-moved to a new commit, a
// lightweight rolling tag, a tag of a tag, and a branch. The snapshot must
// record the commit each tag names (never a tag object) and agree with
// git ls-remote.
func TestAdversarial_Refs_AnnotatedRollingTagRecordsTheCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git CLI not installed")
	}
	dir := t.TempDir()
	gitCLI(t, dir, "init", "-q", "-b", "main")
	gitCLI(t, dir, "commit", "-q", "--allow-empty", "-m", "one")
	first := gitCLI(t, dir, "rev-parse", "HEAD")
	gitCLI(t, dir, "tag", "-a", "nightly", "-m", "first nightly")
	gitCLI(t, dir, "tag", "nightly-latest")
	gitCLI(t, dir, "commit", "-q", "--allow-empty", "-m", "two")
	second := gitCLI(t, dir, "rev-parse", "HEAD")
	gitCLI(t, dir, "tag", "-f", "-a", "nightly", "-m", "second nightly")
	gitCLI(t, dir, "tag", "-f", "nightly-latest")
	gitCLI(t, dir, "tag", "-a", "v1.0.0", "-m", "release", first)
	gitCLI(t, dir, "tag", "-a", "v1.0.0-signed-wrapper", "-m", "tag of a tag", "v1.0.0")

	nightlyObject := gitCLI(t, dir, "rev-parse", "refs/tags/nightly")
	require.NotEqual(t, second, nightlyObject, "an annotated tag is its own object")

	snap, err := newCR(10*time.Second).refsWithCloneURL(context.Background(), dir)
	require.NoError(t, err)

	assert.Equal(t, second, snap.Tags["nightly"], "a moved annotated rolling tag records the commit it now names")
	assert.Equal(t, second, snap.Tags["nightly-latest"])
	assert.Equal(t, first, snap.Tags["v1.0.0"])
	assert.Equal(t, first, snap.Tags["v1.0.0-signed-wrapper"], "a tag of a tag peels to the commit")
	assert.Equal(t, second, snap.Branches["main"])
	assert.Equal(t, "main", snap.Head)
	assert.Equal(t, lsRemotePeeled(t, dir), snap.Tags, "the snapshot agrees with git ls-remote")
}
