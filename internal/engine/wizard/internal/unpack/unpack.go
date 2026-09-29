package unpack

import (
	"fmt"
	"os"
	stdruntime "runtime"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/appimage"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/archive"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/binary"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/dmg"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/msi"
)

type (
	Format = models.Format
	Kind   = models.Kind
	Target = models.Target
	Unit   = models.Unit
	Result = models.Result
	App    = models.App
)

const (
	KindAppImage = models.KindAppImage
	KindDmg      = models.KindDmg
	KindMsi      = models.KindMsi
	KindArchive  = models.KindArchive
	KindBinary   = models.KindBinary

	LauncherName = appimage.LauncherName
)

type Unpacker interface {
	Detect(
		src *os.File,
		size int64,
	) (Format, error)
}

type unpacker struct {
	formats []models.Detect
}

func New(
	maxBytes int64,
) Unpacker {
	return newForOS(maxBytes, stdruntime.GOOS)
}

// newForOS orders the formats most specific first: an AppImage is also an
// ELF executable, and a dmg or msi can pass for nothing else.
func newForOS(
	maxBytes int64,
	goos string,
) Unpacker {
	rules := nameRules(goos)

	return &unpacker{formats: []models.Detect{
		appimage.New(maxBytes, rules),
		dmg.New(maxBytes, rules),
		msi.New(maxBytes, rules),
		archive.New(maxBytes, rules),
		binary.New(maxBytes),
	}}
}

func (u *unpacker) Detect(
	src *os.File,
	size int64,
) (Format, error) {
	for _, detect := range u.formats {
		format, ok, err := detect(src, size)
		if err != nil || ok {
			return format, err
		}
	}

	return nil, fmt.Errorf("unpack: %s: %w", src.Name(), ErrUnknownFormat)
}

func nameRules(
	goos string,
) guard.NameRules {
	switch goos {
	case "windows":
		return guard.NameRules{WindowsNames: true, FoldCase: true}
	case "darwin":
		return guard.NameRules{FoldCase: true}
	default:
		return guard.NameRules{}
	}
}
