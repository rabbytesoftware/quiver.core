package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	stagingSuffix   = ".quiver-tmp"
	ownerMarker     = ".quiver-portable"
	ownerMarkerPerm = 0o644
)

type Installer interface {
	Place(
		ctx context.Context,
		osArch domain.OS,
		workDir string,
		from string,
		to string,
		name string,
	) ([]domain.PortableApp, string, error)
	Record(
		ctx context.Context,
		nsKey string,
		workDir string,
		apps []domain.PortableApp,
	) error
	RemoveSource(
		workDir string,
		from string,
		output string,
	) error
}

type installer struct {
	maxBytes int64
}

func New(
	maxBytes int64,
) Installer {
	return &installer{maxBytes: maxBytes}
}

func (i *installer) Place(
	ctx context.Context,
	osArch domain.OS,
	workDir string,
	from string,
	to string,
	name string,
) ([]domain.PortableApp, string, error) {
	if err := validName(name); err != nil {
		return nil, "", err
	}

	source, owned, err := claim(workDir, from, to)
	if err != nil {
		return nil, "", err
	}
	if !owned {
		return i.install(ctx, osArch, from, to, name, false)
	}

	return i.installOwned(ctx, osArch, from, to, name, source)
}

func (i *installer) RemoveSource(
	workDir string,
	from string,
	output string,
) error {
	if from == output {
		return nil
	}

	if _, inside := workdirRel(workDir, from); !inside {
		return nil
	}

	if err := os.Remove(from); err != nil {
		return fmt.Errorf("portable: remove source %s: %w", from, err)
	}

	return nil
}

func (i *installer) install(
	ctx context.Context,
	osArch domain.OS,
	from string,
	to string,
	name string,
	staged bool,
) ([]domain.PortableApp, string, error) {
	src, err := os.Open(from) // #nosec G304 -- path is the step's own from: field, resolved against the workdir
	if err != nil {
		return nil, "", fmt.Errorf("portable: open %s: %w", from, err)
	}
	defer src.Close() //nolint:errcheck

	info, err := src.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("portable: stat %s: %w", from, err)
	}

	kind, archive, err := detectFormat(src, info.Size())
	if err != nil {
		return nil, "", err
	}

	var apps []domain.PortableApp
	switch kind {
	case formatAppImage:
		apps, err = i.installAppImage(ctx, src, info.Size(), from, to, staged)
		return apps, "", err
	case formatDmg:
		apps, err = i.installDmg(ctx, from, to)
		return apps, "", err
	case formatArchive:
		apps, err = i.installArchive(ctx, osArch, src, info.Size(), archive, to, name)
		return apps, "", err
	case formatExecutable:
		output, err := i.installExecutable(ctx, src, info, from, to, name)
		return nil, output, err
	case formatUnknown:
	}

	return nil, "", fmt.Errorf("%w: %s", ErrUnknownFormat, from)
}

func validName(
	name string,
) error {
	if name == "" || (name != "." && filepath.Base(name) == name && filepath.IsLocal(name)) {
		return nil
	}

	return fmt.Errorf("%w: %q", ErrInvalidName, name)
}

func workdirRel(
	workDir string,
	path string,
) (string, bool) {
	rel, err := filepath.Rel(resolved(workDir), resolvedParent(path))
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return "", false
	}

	return filepath.ToSlash(rel), true
}

func resolvedParent(
	path string,
) string {
	return filepath.Join(resolved(filepath.Dir(path)), filepath.Base(path))
}

func resolved(
	path string,
) string {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}

	return target
}
