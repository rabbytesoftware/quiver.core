package domain

import "time"

type CollectionMeta struct {
	Name        string
	Version     string
	Description string
	URL         string
	Maintainers []string
	Tags        []string
	Media       CollectionMedia
}

type CollectionMedia struct {
	Icon   string
	Banner string
}

// Quiver aggregate — full manifest state + follow state.
type Collection struct {
	Namespace    Namespace
	FollowedAt   time.Time
	FailedArrows []Namespace
	Meta         CollectionMeta
	Arrows       []CollectionArrow
}

// CollectionArrowEntry is the raw translator output before namespace derivation.
// Exactly one of Path or Namespace must be set.
type CollectionArrowEntry struct {
	Path      string `yaml:"path"`
	Namespace string `yaml:"namespace"`
}

// CollectionArrow is a resolved arrow reference with its final namespace.
type CollectionArrow struct {
	Namespace Namespace
}
