package download

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	stagingMarker = ".fetch-"
	asideMarker   = ".replaced-"
	suffixBytes   = 8
)

type renameFn func(oldPath, newPath string) error

// ErrDestinationIsDirectory means a fetch step's "to" names a directory,
// which a download never replaces.
var ErrDestinationIsDirectory = errors.New("download: destination is a directory")

// stagingPath names a file beside dst that no other fetch uses, so a
// download is written, and verified, before it ever replaces dst.
func stagingPath(dst string) (string, error) {
	if info, err := os.Stat(dst); err == nil && info.IsDir() {
		return "", fmt.Errorf("download: %s: %w", dst, ErrDestinationIsDirectory)
	}
	dir, base := filepath.Split(dst)
	return filepath.Join(dir, "."+base+stagingMarker+randomSuffix()), nil
}

// replaceFile moves staged onto dst in one rename, which leaves a process
// executing the old dst running its own image: writing into that file
// instead fails on Linux with "text file busy". Where the OS refuses to
// replace a file in use (a running executable on Windows), the old dst is
// renamed aside first, which it does allow.
func replaceFile(
	staged string,
	dst string,
	rename renameFn,
) error {
	err := rename(staged, dst)
	if err == nil {
		return nil
	}
	aside := dst + asideMarker + randomSuffix()
	if asideErr := rename(dst, aside); asideErr != nil {
		return fmt.Errorf("download: replace %s: %w", dst, err)
	}
	if err := rename(staged, dst); err != nil {
		_ = rename(aside, dst)
		return fmt.Errorf("download: replace %s: %w", dst, err)
	}
	return nil
}

// keepMode gives the staged download the mode of the file it replaces, as
// writing into that file did: an executable fetched again stays executable.
func keepMode(
	staged string,
	dst string,
) {
	info, err := os.Stat(dst)
	if err != nil {
		return
	}
	_ = os.Chmod(staged, info.Mode().Perm())
}

// staleAge is how old a leftover must be before a fetch removes it: longer
// than any download runs, so a concurrent fetch of the same destination never
// loses the staging file it is still writing.
const staleAge = 24 * time.Hour

// sweepStale removes what earlier fetches of dst left behind: a staging file
// of a download that never finished, and old targets moved aside. One still
// in use cannot be removed yet and is left for a later fetch.
func sweepStale(dst string) {
	dir, base := filepath.Split(dst)
	entries, err := os.ReadDir(filepath.Clean(dir))
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-staleAge)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "."+base+stagingMarker) && !strings.HasPrefix(name, base+asideMarker) {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

// randomSuffix never fails: crypto/rand.Read aborts the process rather than
// return an error.
func randomSuffix() string {
	b := make([]byte, suffixBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
