package models

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	ReasonUnsafeName     = "unsafe name"
	ReasonUnsafePath     = "unsafe target path"
	ReasonOutsideWorkdir = "target outside the workdir"
	ReasonNotFound       = "target not found"
	ReasonWrongType      = "target has the wrong file type"
	ReasonNoExecutable   = "no executable found"
	ReasonNoDesktop      = "no desktop application found"
	ReasonAmbiguous      = "ambiguous auto resolution"
	ReasonUnmanaged      = "exists and is not managed by quiver"
	ReasonForeignOwner   = "owned by"
)

type Refusal struct {
	Kind   domain.ExposeKind `json:"kind"`
	Name   string            `json:"name"`
	Reason string            `json:"reason"`
}
