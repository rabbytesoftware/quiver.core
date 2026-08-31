package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── boundedBuffer ───────────────────────────────────────────────────────────

func TestBoundedBuffer_TruncatesAtMax(t *testing.T) {
	buf := newBoundedBuffer(10)
	n, err := buf.Write([]byte("hello world"))
	require.NoError(t, err)
	assert.Equal(t, 11, n, "Write should return input length per io.Writer contract")
	assert.Equal(t, "hello worl", buf.String(), "buffer should truncate at max")
}

func TestBoundedBuffer_SafeToWriteAfterCapacityReached(t *testing.T) {
	buf := newBoundedBuffer(5)
	_, _ = buf.Write([]byte("hello"))
	n, err := buf.Write([]byte(" world"))
	require.NoError(t, err)
	assert.Equal(t, 6, n, "Write should return input length even if buffer is full")
	assert.Equal(t, "hello", buf.String(), "buffer should ignore writes after capacity")
}

func TestBoundedBuffer_StringReturnsWritten(t *testing.T) {
	buf := newBoundedBuffer(100)
	_, _ = buf.Write([]byte("test output"))
	assert.Equal(t, "test output", buf.String())
}

func TestBoundedBuffer_PartialWrite(t *testing.T) {
	buf := newBoundedBuffer(8)
	_, _ = buf.Write([]byte("hello"))
	n, err := buf.Write([]byte("world"))
	require.NoError(t, err)
	assert.Equal(t, 5, n, "Write should return input length per io.Writer contract")
	assert.Equal(t, "hellowor", buf.String(), "only 3 bytes fit in remaining space")
}

func TestBoundedBuffer_EmptyString(t *testing.T) {
	buf := newBoundedBuffer(10)
	assert.Equal(t, "", buf.String())
}

// ─── lastLine ────────────────────────────────────────────────────────────────

func TestLastLine_SingleLineInput(t *testing.T) {
	assert.Equal(t, "error message", lastLine("error message"))
}

func TestLastLine_MultiLineWithLeadingWhitespace(t *testing.T) {
	input := "line 1\nline 2\n  line 3 with indent"
	assert.Equal(t, "line 3 with indent", lastLine(input))
}

// ─── buildDaemonCmd ──────────────────────────────────────────────────────────

// TestBuildDaemonCmd_StderrIsRealFile pins the property the whole SIGPIPE fix
// depends on: os/exec only hands a fd straight to the child, skipping the
// os.Pipe + copier goroutine, when Stderr is an *os.File. Swapping this back
// to a boundedBuffer (or any other io.Writer) reintroduces the pipe, and this
// assertion fails immediately without needing to fork or time anything.
func TestBuildDaemonCmd_StderrIsRealFile(t *testing.T) {
	cmd, path, err := buildDaemonCmd(fakeSelfPath(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(path) })

	f, ok := cmd.Stderr.(*os.File)
	require.True(t, ok, "cmd.Stderr must be a real *os.File, not any other io.Writer")
	assert.Equal(t, path, f.Name())
}

func TestBuildDaemonCmd_ReturnsCreateTempError(t *testing.T) {
	dir := t.TempDir()
	bogus := filepath.Join(dir, "does", "not", "exist")

	t.Setenv("TMPDIR", bogus)

	_, _, err := buildDaemonCmd(fakeSelfPath(t))
	assert.Error(t, err)
}

func fakeSelfPath(t *testing.T) string {
	t.Helper()

	self, err := os.Executable()
	require.NoError(t, err)

	return self
}

// ─── startWith ───────────────────────────────────────────────────────────────

func TestStartWith_PropagatesBuildDaemonCmdError(t *testing.T) {
	dir := t.TempDir()
	bogus := filepath.Join(dir, "does", "not", "exist")

	t.Setenv("TMPDIR", bogus)

	m := &Manager{}
	_, err := m.startWith(fakeSelfPath(t))
	assert.Error(t, err)
}

func TestStartWith_CmdStartFailureLeavesCaptureStderrUnset(t *testing.T) {
	m := &Manager{}
	_, err := m.startWith(filepath.Join(t.TempDir(), "quiver-daemon-does-not-exist"))

	require.Error(t, err)
	assert.Nil(t, m.CaptureStderr, "CaptureStderr must not be wired when the process never started")
	assert.Equal(t, "", m.stderrPath, "stderrPath must not be recorded when the process never started")
}

// ─── readCapturedStderr ──────────────────────────────────────────────────────

func TestReadCapturedStderr_ReturnsFileContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stderr")
	require.NoError(t, os.WriteFile(path, []byte("bind: address already in use\n"), 0o600))

	got := readCapturedStderr(path)

	assert.Equal(t, "bind: address already in use\n", got)
}

func TestReadCapturedStderr_CapsAtLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stderr")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", stderrCaptureLimit+500)), 0o600))

	got := readCapturedStderr(path)

	assert.Len(t, got, stderrCaptureLimit, "a chatty daemon must not grow the captured string past the cap")
}

func TestReadCapturedStderr_MissingFileReturnsEmpty(t *testing.T) {
	got := readCapturedStderr(filepath.Join(t.TempDir(), "missing"))
	assert.Equal(t, "", got)
}

// TestReadCapturedStderr_UnreadablePathReturnsEmpty exercises the read-error
// branch: os.Open succeeds on a directory, but reading from it fails with
// something other than EOF/ErrUnexpectedEOF.
func TestReadCapturedStderr_UnreadablePathReturnsEmpty(t *testing.T) {
	got := readCapturedStderr(t.TempDir())
	assert.Equal(t, "", got)
}

// ─── cleanupStderrFile ───────────────────────────────────────────────────────

func TestCleanupStderrFile_RemovesFileAndResetsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stderr")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

	m := &Manager{stderrPath: path}
	m.cleanupStderrFile()

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "the temp file should be removed")
	assert.Equal(t, "", m.stderrPath)
}

func TestCleanupStderrFile_NoopWhenPathEmpty(t *testing.T) {
	m := &Manager{}
	m.cleanupStderrFile()
	assert.Equal(t, "", m.stderrPath)
}
