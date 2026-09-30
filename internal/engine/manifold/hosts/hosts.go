// Package hosts declares what manifold needs to know about a git host and does
// not own: where the host serves a raw file or a page, which refs it defaults
// to, and what a release publishes.
//
// Manifold owns manifest knowledge — which filenames are a manifest, which refs
// to try, what the bytes mean — and none of that is host knowledge. The
// provider engine implements this contract; the engine container is the only
// place the two ever meet, so neither engine imports the other.
package hosts

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// Host answers the host-specific questions manifest resolution runs into. It
// names URLs and refs, and asks the host only what naming a release's assets
// takes; reading a repository's files is manifold's job.
type Host interface {
	// RawFileURL is where file lives at ref inside the repository ns names.
	RawFileURL(
		ns domain.Namespace,
		ref string,
		file string,
	) (string, error)

	BlobFileURL(
		ns domain.Namespace,
		ref string,
		file string,
	) (string, error)

	RepoPageURL(
		ns domain.Namespace,
	) string

	// DefaultBranches are the refs to try, in order, for a namespace that
	// carries none.
	DefaultBranches() []string

	// An absent or empty release is an empty list; an error means the host
	// could not be asked.
	ReleaseAssets(
		ctx context.Context,
		ns domain.Namespace,
		tag string,
	) ([]domain.ReleaseAsset, error)
}

// Lookup resolves the host serving ns, reporting false when none does. A
// namespace on an unknown host is not an error: it is still reachable by
// cloning, which needs no host knowledge at all.
type Lookup func(ns domain.Namespace) (Host, bool)

// None knows no hosts. It is what manifold falls back to when no lookup is
// wired, so an absent one is a miss rather than a panic.
func None(_ domain.Namespace) (Host, bool) {
	return nil, false
}

// Or falls back to None when lookup is nil.
func Or(lookup Lookup) Lookup {
	if lookup == nil {
		return None
	}
	return lookup
}
