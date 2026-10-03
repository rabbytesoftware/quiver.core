package download_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	stepdownload "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/download"
)

func newTestHandler() wizstep.Handler[domainstep.FetchStep] {
	return stepdownload.NewHandler()
}

func TestHandler_Execute_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	h := newTestHandler()
	dst := filepath.Join(t.TempDir(), "output.txt")
	s := domainstep.NewFetchStep("fetch", srv.URL, dst, "", "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s)

	require.NoError(t, err)
	data, readErr := os.ReadFile(dst)
	require.NoError(t, readErr)
	assert.Equal(t, "hello world", string(data))
}

func TestHandler_Execute_AbsolutePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	h := newTestHandler()
	dst := filepath.Join(t.TempDir(), "file.txt")
	s := domainstep.NewFetchStep("fetch", srv.URL, dst, "", "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: "/other/dir"}, s)

	require.NoError(t, err)
	_, statErr := os.Stat(dst)
	assert.NoError(t, statErr, "file should be at absolute path, not joined with workDir")
}

func TestHandler_Execute_RelativePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	h := newTestHandler()
	workDir := t.TempDir()
	s := domainstep.NewFetchStep("fetch", srv.URL, "file.txt", "", "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: workDir}, s)

	require.NoError(t, err)
	expected := filepath.Join(workDir, "file.txt")
	_, statErr := os.Stat(expected)
	assert.NoError(t, statErr, "file should be at workDir/file.txt")
}

func TestHandler_Execute_VarExpansionInTo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	h := newTestHandler()
	workDir := t.TempDir()
	s := domainstep.NewFetchStep("fetch", srv.URL, "${WORKDIR}/output.txt", "", "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{
		WorkDir: workDir,
		Vars:    map[string]string{"WORKDIR": workDir},
	}, s)

	require.NoError(t, err)
	expected := filepath.Join(workDir, "output.txt")
	_, statErr := os.Stat(expected)
	assert.NoError(t, statErr, "file should be written to the expanded WORKDIR path")
}

func TestHandler_Execute_VarExpansionInURL(t *testing.T) {
	var requested string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	h := newTestHandler()
	dst := filepath.Join(t.TempDir(), "out.txt")
	s := domainstep.NewFetchStep("fetch", srv.URL+"/download/${REF}/asset.tgz", dst, "", "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{
		WorkDir: "/tmp",
		Vars:    map[string]string{"REF": "v1.2.0"},
	}, s)

	require.NoError(t, err)
	assert.Equal(t, "/download/v1.2.0/asset.tgz", requested)
}

// TestHandler_Execute_UnknownVarInToLeftVerbatim pins the replacement of
// os.Expand: an unknown reference must stay visible in the path rather than
// collapse to an empty string.
func TestHandler_Execute_UnknownVarInToLeftVerbatim(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	h := newTestHandler()
	workDir := t.TempDir()
	s := domainstep.NewFetchStep("fetch", srv.URL, "${NOT_A_QUIVER_VAR}.txt", "", "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: workDir}, s)

	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(workDir, "${NOT_A_QUIVER_VAR}.txt"))
	assert.NoError(t, statErr, "unknown reference must stay verbatim in the path")
}

func TestHandler_Execute_ShellFormInToLeftVerbatim(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	h := newTestHandler()
	workDir := t.TempDir()
	s := domainstep.NewFetchStep("fetch", srv.URL, "$HOME.txt", "", "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{
		WorkDir: workDir,
		Vars:    map[string]string{"HOME": "/should/not/be/used"},
	}, s)

	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(workDir, "$HOME.txt"))
	assert.NoError(t, statErr, "bare $NAME belongs to the shell and must not be expanded")
}

func TestHandler_Execute_InvalidTimeout_ReturnsError(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte("x"))
	}))
	t.Cleanup(srv.Close)
	workDir := t.TempDir()
	h := newTestHandler()
	s := domainstep.NewFetchStep("fetch", srv.URL+"/x", "out.txt", "", "bad-timeout", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: workDir}, s)

	require.Error(t, err)
	assert.Zero(t, hits, "a step with an unusable timeout must not start the download")
	assert.NoFileExists(t, filepath.Join(workDir, "out.txt"))
}

func TestHandler_Execute_DownloadError(t *testing.T) {
	h := newTestHandler()
	s := domainstep.NewFetchStep("fetch", "http://127.0.0.1:0/nonexistent", "/tmp/out.txt", "", "5s", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s)

	require.Error(t, err)
}

func TestHandler_Execute_NonHTTPURL_ReturnsClearError(t *testing.T) {
	testCases := []struct {
		name string
		url  string
	}{
		{name: "empty", url: ""},
		{name: "local path", url: "/etc/hosts"},
		{name: "file scheme", url: "file:///etc/hosts"},
		{name: "ftp scheme", url: "ftp://example.com/quiver"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "out.bin")
			s := domainstep.NewFetchStep("fetch", tc.url, dst, "", "5s", true)

			err := newTestHandler().Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s)

			require.ErrorIs(t, err, stepdownload.ErrUnsupportedURL)
			assert.Contains(t, err.Error(), "http or https")
			assert.NoFileExists(t, dst)
		})
	}
}

func TestHandler_Execute_TruncatedBody_LeavesNoFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("short"))
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.bin")
	sum := sha256.Sum256([]byte("short"))
	s := domainstep.NewFetchStep("fetch", srv.URL, dst, hex.EncodeToString(sum[:]), "5s", true)

	err := newTestHandler().Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s)

	require.Error(t, err)
	assert.NoFileExists(t, dst)
}

func TestHandler_Execute_Timeout(t *testing.T) {
	// Use a handler that blocks with context-aware timeout
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block for longer than the test timeout will allow
		select {
		case <-time.After(10 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			// Context cancelled - the download handler should have cancelled it
			http.Error(w, "context cancelled", http.StatusRequestTimeout)
		}
	}))
	defer slow.Close()

	h := newTestHandler()
	dst := filepath.Join(t.TempDir(), "out.txt")
	s := domainstep.NewFetchStep("fetch", slow.URL, dst, "", "50ms", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s)

	require.Error(t, err)
}

func TestHandler_Execute_ChecksumMatch_Success(t *testing.T) {
	content := []byte("release binary contents")
	sum := sha256.Sum256(content)
	expected := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	h := newTestHandler()
	dst := filepath.Join(t.TempDir(), "out.bin")
	s := domainstep.NewFetchStep("fetch", srv.URL, dst, expected, "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s)

	require.NoError(t, err)
	data, readErr := os.ReadFile(dst)
	require.NoError(t, readErr)
	assert.Equal(t, content, data)
}

func TestHandler_Execute_ChecksumMismatch_ReturnsErrorAndRemovesFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("actual content"))
	}))
	defer srv.Close()

	h := newTestHandler()
	dst := filepath.Join(t.TempDir(), "out.bin")
	s := domainstep.NewFetchStep("fetch", srv.URL, dst, "0000000000000000000000000000000000000000000000000000000000000000", "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s)

	require.Error(t, err)
	assert.ErrorIs(t, err, stepdownload.ErrChecksumMismatch)
	_, statErr := os.Stat(dst)
	assert.True(t, os.IsNotExist(statErr), "mismatched download must be removed, not left on disk")
}

func TestHandler_Execute_ChecksumCaseInsensitive(t *testing.T) {
	content := []byte("case test content")
	sum := sha256.Sum256(content)
	expected := strings.ToUpper(hex.EncodeToString(sum[:]))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	h := newTestHandler()
	dst := filepath.Join(t.TempDir(), "out.bin")
	s := domainstep.NewFetchStep("fetch", srv.URL, dst, expected, "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s)

	require.NoError(t, err)
}

// TestHandler_Execute_ChecksumVarExpansion pins the same expansion dst and
// url already get (TestHandler_Execute_VarExpansionInTo/URL above) for the
// checksum field too: a manifest that declares checksum as a variable
// reference (e.g. self-update's "${QUIVER_RELEASE_CHECKSUM}", assembled by
// BeginUpdate at execution time rather than baked into the manifest) must
// have that reference resolved before comparison — not compared against the
// literal, unexpanded "${...}" text.
func TestHandler_Execute_ChecksumVarExpansion(t *testing.T) {
	content := []byte("release payload")
	sum := sha256.Sum256(content)
	expected := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	h := newTestHandler()
	dst := filepath.Join(t.TempDir(), "out.bin")
	s := domainstep.NewFetchStep("fetch", srv.URL, dst, "${RELEASE_CHECKSUM}", "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{
		WorkDir: "/tmp",
		Vars:    map[string]string{"RELEASE_CHECKSUM": expected},
	}, s)

	require.NoError(t, err)
	data, readErr := os.ReadFile(dst)
	require.NoError(t, readErr)
	assert.Equal(t, content, data)
}

// TestHandler_Execute_ChecksumVarResolvesEmpty_ReturnsErrorAndRemovesFile
// guards the security gap a naive fix for the above would open: a checksum
// declared as a variable reference (contains "${") must error, not silently
// skip verification, when that reference resolves to an empty string. Only a
// manifest whose checksum field is genuinely absent (the raw, undeclared
// field is already "") may skip — see TestHandler_Execute_Success, whose
// checksum is "" with no "${" in it at all.
func TestHandler_Execute_ChecksumVarResolvesEmpty_ReturnsErrorAndRemovesFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("actual content"))
	}))
	defer srv.Close()

	h := newTestHandler()
	dst := filepath.Join(t.TempDir(), "out.bin")
	s := domainstep.NewFetchStep("fetch", srv.URL, dst, "${MISSING_CHECKSUM}", "10s", true)

	err := h.Execute(context.Background(), wizstep.Request{
		WorkDir: "/tmp",
		Vars:    map[string]string{"MISSING_CHECKSUM": ""},
	}, s)

	require.Error(t, err)
	assert.ErrorIs(t, err, stepdownload.ErrChecksumUnresolved)
	_, statErr := os.Stat(dst)
	assert.True(t, os.IsNotExist(statErr), "a download whose declared checksum never resolved must be removed, not left on disk")
}

func TestHandler_Execute_ChecksumAlgorithmPrefix(t *testing.T) {
	content := []byte("algorithm tagged content")
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	zeros := strings.Repeat("0", 64)

	testCases := []struct {
		name     string
		checksum string
		wantErr  error
	}{
		{name: "bare hex matches", checksum: digest},
		{name: "lowercase sha256 prefix matches", checksum: "sha256:" + digest},
		{name: "uppercase sha256 prefix matches", checksum: "SHA256:" + strings.ToUpper(digest)},
		{name: "bare hex with surrounding whitespace matches", checksum: " " + digest + "\n"},
		{name: "sha256 prefix with surrounding whitespace matches", checksum: "\tsha256:" + digest + " "},
		{name: "sha256 prefix mismatch", checksum: "sha256:" + zeros, wantErr: stepdownload.ErrChecksumMismatch},
		{name: "sha512 prefix unsupported", checksum: "sha512:" + digest, wantErr: stepdownload.ErrUnsupportedChecksumAlgorithm},
		{name: "md5 prefix unsupported", checksum: "md5:" + zeros[:32], wantErr: stepdownload.ErrUnsupportedChecksumAlgorithm},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(content)
			}))
			defer srv.Close()

			dst := filepath.Join(t.TempDir(), "out.bin")
			s := domainstep.NewFetchStep("fetch", srv.URL, dst, tc.checksum, "10s", true)

			err := newTestHandler().Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s)

			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
			assert.Equal(t, errors.Is(tc.wantErr, stepdownload.ErrChecksumMismatch), errors.Is(err, stepdownload.ErrChecksumMismatch))
			_, statErr := os.Stat(dst)
			assert.True(t, os.IsNotExist(statErr))
		})
	}
}

// quiver.core's second self-update in one daemon lifetime downloads over the
// binary the first one exec'd and is still running. Writing into a running
// executable fails on Linux with "text file busy"; the fetch must replace it
// instead, leaving the running process on its own image.
func TestHandler_Execute_ReplacesARunningExecutable(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("no /bin/sleep; the Windows move-aside path is covered by TestReplaceFile")
	}
	sleepBin, err := exec.LookPath("sleep")
	require.NoError(t, err)
	running, err := os.ReadFile(sleepBin) // #nosec G304 -- the system's own sleep binary
	require.NoError(t, err)
	dst := filepath.Join(t.TempDir(), "quiver-new")
	require.NoError(t, os.WriteFile(dst, running, 0o700)) // #nosec G306 -- it must be executable to run
	cmd := exec.Command(dst, "30")                        // #nosec G204 -- a temp copy of sleep this test owns
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	next := []byte("the next build")
	sum := sha256.Sum256(next)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(next)
	}))
	defer srv.Close()
	s := domainstep.NewFetchStep("fetch", srv.URL, dst, hex.EncodeToString(sum[:]), "10s", true)

	for range 2 {
		require.NoError(t, newTestHandler().Execute(context.Background(), wizstep.Request{WorkDir: filepath.Dir(dst)}, s))
	}

	got, err := os.ReadFile(dst) // #nosec G304 -- a temp file this test owns
	require.NoError(t, err)
	assert.Equal(t, next, got)
	assert.Nil(t, cmd.ProcessState, "the running process keeps its own image")
	entries, err := os.ReadDir(filepath.Dir(dst))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no staging file is left behind")
}

// A download that fails its checksum never reaches the destination: what was
// there before (the build that is running) stays.
func TestHandler_Execute_ChecksumMismatch_KeepsThePreviousFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("tampered"))
	}))
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), "quiver-new")
	require.NoError(t, os.WriteFile(dst, []byte("previous"), 0o600))
	s := domainstep.NewFetchStep("fetch", srv.URL, dst, strings.Repeat("0", 64), "10s", true)

	err := newTestHandler().Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s)

	require.ErrorIs(t, err, stepdownload.ErrChecksumMismatch)
	got, readErr := os.ReadFile(dst) // #nosec G304 -- a temp file this test owns
	require.NoError(t, readErr)
	assert.Equal(t, "previous", string(got))
	entries, dirErr := os.ReadDir(filepath.Dir(dst))
	require.NoError(t, dirErr)
	assert.Len(t, entries, 1, "the rejected download is not left beside it")
}

func TestHandler_Execute_DirectoryDestination_IsRefused(t *testing.T) {
	s := domainstep.NewFetchStep("fetch", "http://127.0.0.1:1/never", t.TempDir(), "", "10s", true)

	err := newTestHandler().Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s)

	require.ErrorIs(t, err, stepdownload.ErrDestinationIsDirectory)
}

// An arrow that made its binary executable at install and fetches it again at
// update keeps it executable: the replacement takes the old file's mode.
func TestHandler_Execute_RefetchKeepsTheDestinationsMode(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("windows has no executable bit")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("next"))
	}))
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), "tool")
	require.NoError(t, os.WriteFile(dst, []byte("previous"), 0o700)) // #nosec G306 -- an executable this test owns
	s := domainstep.NewFetchStep("fetch", srv.URL, dst, "", "10s", true)

	require.NoError(t, newTestHandler().Execute(context.Background(), wizstep.Request{WorkDir: "/tmp"}, s))

	info, err := os.Stat(dst)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}
