package models

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type Exposer interface {
	Find(
		req Request,
		entry domain.ExposeEntry,
	) ([]Candidate, string, error)
	Place(
		ctx context.Context,
		req Request,
		entry domain.ExposeEntry,
		c Candidate,
	) (Placement, error)
	Remove(
		ctx context.Context,
		l Layout,
		claim Claim,
		keep map[string]bool,
	) error
}

type PathManager interface {
	Status(
		ctx context.Context,
	) (PathStatus, error)
	Setup(
		ctx context.Context,
	) (PathStatus, error)
}

type PathStatus struct {
	BinDir     string
	OnPath     bool
	Configured bool
	Files      []string
}

type Claim struct {
	Bare    domain.Namespace
	Workdir string
}

func NamespaceClaim(
	bare domain.Namespace,
) Claim {
	return Claim{Bare: bare}
}

func WorkdirClaim(
	bare domain.Namespace,
	workdir string,
) Claim {
	return Claim{Bare: bare, Workdir: workdir}
}
