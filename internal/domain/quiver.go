package domain

import "time"

// Quiver aggregate — stores only follow state; manifest lives in vault.
type Quiver struct {
	Namespace  Namespace `json:"namespace"`
	FollowedAt time.Time `json:"followed_at"`
}

// QuiverArrowEntry is the raw translator output before namespace derivation.
// Exactly one of Path or Namespace must be set.
type QuiverArrowEntry struct {
	Path      string `yaml:"path"`
	Namespace string `yaml:"namespace"`
}

// QuiverArrow is a resolved arrow reference with its final namespace.
type QuiverArrow struct {
	Namespace Namespace
}

type QuiverManifest struct {
	Name        string
	Version     string
	Description string
	URL         string
	Maintainers []string
	Tags        []string
	Media       QuiverMedia
	Arrows      []QuiverArrow
}

type QuiverMedia struct {
	Icon   string
	Banner string
}
