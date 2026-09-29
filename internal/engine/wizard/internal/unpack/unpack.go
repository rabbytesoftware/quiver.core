package unpack

import (
	"context"
	"io"
	"os"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/appimage"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/archive"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/dmg"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
)

const LauncherName = appimage.LauncherName

type (
	Guard        = guard.Guard
	GuardOption  = guard.Option
	Archive      = archive.Archive
	AppImageMeta = appimage.Meta
)

func OpenGuard(
	ctx context.Context,
	dest string,
	maxBytes int64,
	opts ...GuardOption,
) (*Guard, error) {
	return guard.Open(ctx, dest, maxBytes, opts...)
}

func SkipEscapingLinks() GuardOption {
	return guard.SkipEscapingLinks()
}

func DetectArchive(
	src *os.File,
	size int64,
) (Archive, error) {
	return archive.Detect(src, size)
}

func IsAppImage(
	src io.ReaderAt,
) bool {
	return appimage.Is(src)
}

func IsDmg(
	src io.ReaderAt,
	size int64,
) bool {
	return dmg.Is(src, size)
}

func ExtractAppImage(
	ctx context.Context,
	src *os.File,
	size int64,
	g *Guard,
) error {
	return appimage.Extract(ctx, src, size, g)
}

func ExtractDmg(
	ctx context.Context,
	image string,
	g *Guard,
) error {
	return dmg.Extract(ctx, image, g)
}

func AppDirName(
	from string,
) string {
	return appimage.DirName(from)
}

func ReadAppImageMeta(
	appDir string,
) (AppImageMeta, error) {
	return appimage.ReadMeta(appDir)
}

func WriteLauncher(
	appDir string,
	args []string,
) (string, error) {
	return appimage.WriteLauncher(appDir, args)
}
