package domain

import "time"

type QuiverMeta struct {
	Name        string
	Version     string
	Description string
	URL         string
	Maintainers []string
	Tags        []string
	Media       QuiverMedia
}

type QuiverMedia struct {
	Icon   string
	Banner string
}

// Quiver aggregate — full manifest state + follow state.
type Quiver struct {
	Namespace    Namespace
	FollowedAt   time.Time
	FailedArrows []Namespace
	Meta         QuiverMeta
	Arrows       []QuiverArrow
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
