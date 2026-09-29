package models

import (
	"context"
	"os"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type Kind string

const (
	KindAppImage Kind = "appimage"
	KindDmg      Kind = "dmg"
	KindMsi      Kind = "msi"
	KindArchive  Kind = "archive"
	KindBinary   Kind = "binary"
)

type App = domain.PortableApp

type Format interface {
	Kind() Kind
	Unit() Unit
	Unpack(
		ctx context.Context,
		target Target,
	) (Result, error)
}

type Detect func(
	src *os.File,
	size int64,
) (Format, bool, error)

type Target struct {
	Dir  string
	Name string
}

// Unit is the directory below the destination that holds a format's whole
// output and nothing else, so it can be staged and replaced on its own. The
// zero Unit means the output merges into the destination. Proof names the
// file whose presence marks an existing Unit directory as a previous output.
type Unit struct {
	Dir   string
	Proof string
}

type Result struct {
	Apps   []App
	Output string
}
