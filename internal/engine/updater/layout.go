package updater

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// SystemNamespace identifies the protected built-in Quiver Arrow.
const SystemNamespace = "github.com/rabbytesoftware/quiver"

// MaxVersionLength keeps the content-addressed directory name portable.
const MaxVersionLength = 64

// Layout contains the trusted filesystem roots used by self-update state.
type Layout struct {
	namespaces     string
	root           string
	versions       string
	update         string
	executableName string
}

// NewLayout resolves the protected Quiver Arrow update layout below namespacesPath.
func NewLayout(namespacesPath string) (Layout, error) {
	return newLayout(namespacesPath, runtime.GOOS)
}

func newLayout(namespacesPath, goos string) (Layout, error) {
	if namespacesPath == "" || !filepath.IsAbs(namespacesPath) {
		return Layout{}, fmt.Errorf("updater layout: namespaces path must be absolute: %w", ErrInvalidLayout)
	}
	if goos == "" {
		return Layout{}, fmt.Errorf("updater layout: operating system is required: %w", ErrInvalidLayout)
	}

	root := filepath.Join(namespacesPath, filepath.FromSlash(SystemNamespace))
	executableName := "quiver"
	if goos == "windows" {
		executableName += ".exe"
	}

	return Layout{
		namespaces:     filepath.Clean(namespacesPath),
		root:           root,
		versions:       filepath.Join(root, "versions"),
		update:         filepath.Join(root, "update"),
		executableName: executableName,
	}, nil
}

// Root returns the protected Quiver Arrow workdir used by the updater.
func (l Layout) Root() string {
	return l.root
}

// VersionsDir returns the immutable version store directory.
func (l Layout) VersionsDir() string {
	return l.versions
}

// UpdateDir returns the directory containing update transaction state.
func (l Layout) UpdateDir() string {
	return l.update
}

// CurrentPath returns the active selection pointer path.
func (l Layout) CurrentPath() string {
	if l.validate() != nil {
		return ""
	}
	return filepath.Join(l.update, "current.json")
}

// StagedPath returns the verified candidate pointer path.
func (l Layout) StagedPath() string {
	if l.validate() != nil {
		return ""
	}
	return filepath.Join(l.update, "staged.json")
}

// AttemptPath returns the in-flight activation record path.
func (l Layout) AttemptPath() string {
	if l.validate() != nil {
		return ""
	}
	return filepath.Join(l.update, "attempt.json")
}

func (l Layout) artifact(version, digest string) (Artifact, error) {
	if err := l.validate(); err != nil {
		return Artifact{}, err
	}
	if err := validateVersion(version); err != nil {
		return Artifact{}, err
	}
	if err := validateDigest(digest); err != nil {
		return Artifact{}, err
	}

	dirName := version + "-" + digest
	relative := filepath.ToSlash(filepath.Join("versions", dirName, l.executableName))
	return Artifact{Version: version, Digest: digest, Executable: relative}, nil
}

func validateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("updater version: value is required: %w", ErrInvalidState)
	}
	if len(version) > MaxVersionLength {
		return fmt.Errorf("updater version: exceeds %d bytes: %w", MaxVersionLength, ErrInvalidState)
	}
	for _, char := range version {
		if isVersionChar(char) {
			continue
		}
		return fmt.Errorf("updater version %q: unsupported character: %w", version, ErrInvalidState)
	}
	return nil
}

func (l Layout) validate() error {
	paths := []string{l.namespaces, l.root, l.versions, l.update}
	for _, path := range paths {
		if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("updater layout: invalid absolute path: %w", ErrInvalidLayout)
		}
	}

	wantRoot := filepath.Join(l.namespaces, filepath.FromSlash(SystemNamespace))
	if l.root != wantRoot || l.versions != filepath.Join(wantRoot, "versions") || l.update != filepath.Join(wantRoot, "update") {
		return fmt.Errorf("updater layout: roots are inconsistent: %w", ErrInvalidLayout)
	}
	if l.executableName != "quiver" && l.executableName != "quiver.exe" {
		return fmt.Errorf("updater layout: invalid executable name: %w", ErrInvalidLayout)
	}
	return nil
}

func isVersionChar(char rune) bool {
	return char >= 'a' && char <= 'z' ||
		char >= 'A' && char <= 'Z' ||
		char >= '0' && char <= '9' ||
		char == '.' || char == '-' || char == '_' || char == '+'
}

func validateDigest(digest string) error {
	if len(digest) != 64 {
		return fmt.Errorf("updater digest: expected 64 lowercase hexadecimal characters: %w", ErrInvalidState)
	}
	for _, char := range digest {
		if char >= '0' && char <= '9' || char >= 'a' && char <= 'f' {
			continue
		}
		return fmt.Errorf("updater digest: expected lowercase hexadecimal: %w", ErrInvalidState)
	}
	return nil
}
