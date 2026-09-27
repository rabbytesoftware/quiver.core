package shelf

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	reasonUnsafeName     = "unsafe name"
	reasonUnsafePath     = "unsafe target path"
	reasonOutsideWorkdir = "target outside the workdir"
	reasonNotFound       = "target not found"
	reasonWrongType      = "target has the wrong file type"
	reasonNoExecutable   = "no executable found"
	reasonNoDesktop      = "no desktop application found"
	reasonAmbiguous      = "ambiguous auto resolution"
	reasonUnmanaged      = "exists and is not managed by quiver"
	reasonForeignOwner   = "owned by"
)

type Refusal struct {
	Kind   domain.ExposeKind `json:"kind"`
	Name   string            `json:"name"`
	Reason string            `json:"reason"`
}
